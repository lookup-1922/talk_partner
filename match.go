// match.go
//
// マッチング処理: 文字列類似度の計算と、BM25＋リランカーの2相パイプライン。
package main

import "unicode/utf8"

const (
	bm25TopK        = 5    // 第1相で残す候補数
	rerankThreshold = 0.40 // 第2相のスコアがこれ未満ならfallback
)

const fallbackReply = "ごめん、よくわからなかった…もう一回言ってくれる？"
const fallbackExpression = "normal"

// MatchResult は findBestResponse の結果。
type MatchResult struct {
	Reply      string  `json:"reply"`
	Expression string  `json:"expression"`
	Score      float64 `json:"-"`
}

// similarity は2つの文字列の類似度を 0.0〜1.0 で返す。
// 正規化されたレーベンシュタイン距離を用いる（日本語のrune単位で比較）。
func similarity(a, b string) float64 {
	ra := []rune(a)
	rb := []rune(b)

	dist := levenshtein(ra, rb)
	maxLen := utf8.RuneCountInString(a)
	if l := utf8.RuneCountInString(b); l > maxLen {
		maxLen = l
	}
	if maxLen == 0 {
		return 0
	}
	return 1.0 - float64(dist)/float64(maxLen)
}

// levenshtein は2つのrune列の編集距離を計算する。
func levenshtein(a, b []rune) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			curr[j] = min3(del, ins, sub)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}

// findBestResponse は BM25 による第1相の絞り込みと、
// リランカー（クロスエンコーダ相当）による第2相の精査を組み合わせて
// 最良の応答を選ぶ。どの候補も閾値未満ならfallbackを返す。
func findBestResponse(
	input string,
	entries []ResponseEntry,
	bm25Index *BM25Index,
	reranker CrossEncoderScorer,
) MatchResult {
	// 第1相: BM25で候補を topK 件に絞り込む
	candidates := bm25Index.Search(input, bm25TopK)
	if len(candidates) == 0 {
		return MatchResult{Reply: fallbackReply, Expression: fallbackExpression}
	}

	maxBM25 := candidates[0].Score // Search はスコア降順で返す

	// 第2相: 絞り込んだ候補だけをクエリとのペアで精査し、並べ替える
	best := MatchResult{Score: 0}
	for _, cand := range candidates {
		entry := entries[cand.EntryIndex]
		s := reranker.Score(input, entry, cand, maxBM25)
		if s > best.Score {
			best = MatchResult{
				Reply:      entry.Reply,
				Expression: entry.NormalizedExpression(),
				Score:      s,
			}
		}
	}

	if best.Score >= rerankThreshold {
		return best
	}
	return MatchResult{Reply: fallbackReply, Expression: fallbackExpression, Score: best.Score}
}
