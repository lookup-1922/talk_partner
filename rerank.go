// rerank.go
//
// 第2相: 精密なランキング（クロスエンコーダ相当）
//
// 本来のクロスエンコーダは「クエリと候補を1つのニューラルネットに
// 同時に入力し、文脈込みで関連度を出す」手法だが、事前学習済み
// モデルを使うには重い依存（transformers / ONNX Runtime + モデル
// ファイル数百MB）が必要になり、部報が想定していた
// 「軽量なローカル動作」という制約と衝突する。
//
// そこで、役割（＝BM25で絞った少数の候補だけを、クエリとの
// ペアで詳細に見て並べ替える）はそのまま踏襲しつつ、
// 実装は複数の統計的特徴量を組み合わせた軽量スコアラーで代用する。
//
// CrossEncoderScorer インターフェースにしてあるので、将来的に
// 本物のニューラルモデルへ差し替える場合はこのファイルの実装を
// 入れ替えるだけでよい。
package main

import "unicode/utf8"

// CrossEncoderScorer はクエリと候補文のペアを受け取り、
// 関連度スコア（大きいほど関連度が高い、目安として0.0〜1.0）を返す。
type CrossEncoderScorer interface {
	Score(query string, entry ResponseEntry, bm25Candidate BM25Candidate, maxBM25Score float64) float64
}

// LexicalCrossEncoder は複数の文字列類似度特徴量を組み合わせた
// 軽量リランカー。ニューラルネットではないが、候補ごとにクエリとの
// ペアで複数の特徴を同時に評価する点は本来のクロスエンコーダの
// 役割を踏襲している。
type LexicalCrossEncoder struct{}

func (LexicalCrossEncoder) Score(query string, entry ResponseEntry, cand BM25Candidate, maxBM25Score float64) float64 {
	// 特徴量1: BM25スコアの正規化値（第1相の情報も引き継ぐ）
	//
	// 候補間の相対値（cand.Score / maxBM25Score）で正規化すると、
	// 候補群の中で一番マシなだけの弱い一致でも常に1.0になってしまい、
	// 「そもそも全候補と無関係な入力」を締め出せなくなる。
	// そのため絶対スコアに対する飽和正規化（0に近いほど0、大きいほど1に漸近）を使う。
	const bm25Saturation = 2.0 // この値付近のBM25スコアで正規化値がおよそ0.67になる
	normalizedBM25 := cand.Score / (cand.Score + bm25Saturation)

	// 特徴量2: 最良triggerとの編集距離ベースの類似度
	bestEditSim := 0.0
	for _, trig := range entry.Triggers {
		if s := similarity(query, trig); s > bestEditSim {
			bestEditSim = s
		}
	}

	// 特徴量3: bi-gram のJaccard類似度（BM25とは別の角度で語の重なりを見る）
	bestJaccard := 0.0
	queryGrams := uniqueSet(tokenize(query))
	for _, trig := range entry.Triggers {
		j := jaccard(queryGrams, uniqueSet(tokenize(trig)))
		if j > bestJaccard {
			bestJaccard = j
		}
	}

	// 特徴量4: 文字数の比率が近いほど加点（極端に長さが違う文はノイズになりやすい）
	lengthPenalty := lengthRatio(query, cand.BestTrigger)

	// 重み付き合成。BM25で粗く絞ったあと、編集距離とn-gram重なりで
	// 精密に並べ替えるイメージ。
	score := 0.40*normalizedBM25 + 0.35*bestEditSim + 0.20*bestJaccard + 0.05*lengthPenalty
	return score
}

func jaccard(a, b map[string]struct{}) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	intersection := 0
	for t := range a {
		if _, ok := b[t]; ok {
			intersection++
		}
	}
	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func lengthRatio(a, b string) float64 {
	la := utf8.RuneCountInString(a)
	lb := utf8.RuneCountInString(b)
	if la == 0 || lb == 0 {
		return 0
	}
	if la > lb {
		la, lb = lb, la
	}
	return float64(la) / float64(lb)
}
