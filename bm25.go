// bm25.go
//
// 第1相: BM25による高速絞り込み
//
// 日本語は分かち書きがないため、形態素解析器を使わずに
// 文字bi-gram（2文字の連続）をトークンとして扱う。
// これにより外部辞書・外部ライブラリなしでBM25を適用できる。
package main

import (
	"math"
	"sort"
)

const (
	bm25K1 = 1.5
	bm25B  = 0.75
)

// bm25Doc はBM25インデックス上の1文書（=1つのtrigger例文）を表す。
type bm25Doc struct {
	entryIndex int      // 対応する ResponseEntry のインデックス
	trigger    string   // 元の文字列（デバッグ用）
	tokens     []string // トークン列
	length     int      // トークン数
}

// BM25Index はtrigger例文の集合に対するBM25検索インデックス。
type BM25Index struct {
	docs  []bm25Doc
	df    map[string]int // トークンごとの文書頻度 (document frequency)
	avgdl float64        // 平均文書長
	n     int             // 総文書数
}

// tokenize は文字列を bi-gram のトークン列に分割する。
// rune数が1の場合はその1文字をそのままトークンとする。
func tokenize(s string) []string {
	r := []rune(s)
	if len(r) == 0 {
		return nil
	}
	if len(r) == 1 {
		return []string{string(r)}
	}
	tokens := make([]string, 0, len(r)-1)
	for i := 0; i < len(r)-1; i++ {
		tokens = append(tokens, string(r[i:i+2]))
	}
	return tokens
}

// buildBM25Index は応答データセットからBM25インデックスを構築する。
// エントリが持つtriggerごとに1文書として登録する。
func buildBM25Index(entries []ResponseEntry) *BM25Index {
	idx := &BM25Index{
		df: make(map[string]int),
	}

	totalLen := 0
	for entryIdx, e := range entries {
		for _, trig := range e.Triggers {
			tokens := tokenize(trig)
			doc := bm25Doc{
				entryIndex: entryIdx,
				trigger:    trig,
				tokens:     tokens,
				length:     len(tokens),
			}
			idx.docs = append(idx.docs, doc)
			totalLen += doc.length

			for t := range uniqueSet(tokens) {
				idx.df[t]++
			}
		}
	}

	idx.n = len(idx.docs)
	if idx.n > 0 {
		idx.avgdl = float64(totalLen) / float64(idx.n)
	}
	return idx
}

func uniqueSet(tokens []string) map[string]struct{} {
	set := make(map[string]struct{}, len(tokens))
	for _, t := range tokens {
		set[t] = struct{}{}
	}
	return set
}

// idf は Okapi BM25 の標準的なIDF計算式。
// n_q が N の半数を超える極めてありふれたトークンでも
// スコアが負にならないよう +1 する変種を用いる。
func (idx *BM25Index) idf(token string) float64 {
	nq := idx.df[token]
	if nq == 0 {
		return 0
	}
	return math.Log(float64(idx.n-nq)+0.5) - math.Log(float64(nq)+0.5) + 1
}

func (idx *BM25Index) scoreDoc(queryTokens []string, doc bm25Doc) float64 {
	if idx.avgdl == 0 {
		return 0
	}

	tf := make(map[string]int, len(doc.tokens))
	for _, t := range doc.tokens {
		tf[t]++
	}

	score := 0.0
	for t := range uniqueSet(queryTokens) {
		idf := idx.idf(t)
		if idf <= 0 {
			continue
		}
		f := float64(tf[t])
		numerator := f * (bm25K1 + 1)
		denominator := f + bm25K1*(1-bm25B+bm25B*float64(doc.length)/idx.avgdl)
		if denominator == 0 {
			continue
		}
		score += idf * numerator / denominator
	}
	return score
}

// BM25Candidate は第1相の検索結果1件分。
type BM25Candidate struct {
	EntryIndex int
	BestTrigger string
	Score      float64
}

// Search はクエリに対してBM25スコア上位 topK 件のエントリを返す。
// 同一エントリに複数triggerがヒットした場合は最良スコアを採用する。
func (idx *BM25Index) Search(query string, topK int) []BM25Candidate {
	queryTokens := tokenize(query)
	if len(queryTokens) == 0 {
		return nil
	}

	best := make(map[int]BM25Candidate)
	for _, doc := range idx.docs {
		s := idx.scoreDoc(queryTokens, doc)
		if s <= 0 {
			continue
		}
		cur, ok := best[doc.entryIndex]
		if !ok || s > cur.Score {
			best[doc.entryIndex] = BM25Candidate{
				EntryIndex:  doc.entryIndex,
				BestTrigger: doc.trigger,
				Score:       s,
			}
		}
	}

	candidates := make([]BM25Candidate, 0, len(best))
	for _, c := range best {
		candidates = append(candidates, c)
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	if topK > 0 && len(candidates) > topK {
		candidates = candidates[:topK]
	}
	return candidates
}
