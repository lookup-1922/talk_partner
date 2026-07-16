// 話し相手プログラム（Go版）
//
// 部報80号「話し相手をプログラミング！」の設計をベースに実装。
//   - 応答生成: 事前に用意した応答セット（responses.json）から選択
//   - マッチング: 2相式
//                第1相: BM25（bm25.go）でtopK件まで高速に絞り込み
//                第2相: 軽量リランカー（rerank.go, クロスエンコーダ役）
//                       で候補を精査し、最良の1件を選ぶ
//   - 音声: VOICEVOXのローカルサーバー(localhost:50021)にHTTPで問い合わせる
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	voicevoxBaseURL      = "http://localhost:50021"
	defaultSpeakerID     = 108 // 東北きりたん
	defaultOutputWavPath = "output.wav"
	responsesFilePath    = "responses.json"

	bm25TopK        = 5    // 第1相で残す候補数
	rerankThreshold = 0.40 // 第2相のスコアがこれ未満ならfallback
)

const fallbackReply = "ごめん、よくわからなかった…もう一回言ってくれる？"

// ResponseEntry は応答データセット1件分の構造。
// triggers には言い回しのバリエーションを複数入れられる。
type ResponseEntry struct {
	ID       string   `json:"id"`
	Triggers []string `json:"triggers"`
	Note     string   `json:"note"`
	Reply    string   `json:"reply"`
}

// loadResponses は JSON ファイルから応答データセットを読み込む。
func loadResponses(path string) ([]ResponseEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("データセットの読み込みに失敗しました: %w", err)
	}

	var entries []ResponseEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("データセットのパースに失敗しました: %w", err)
	}

	// reply が未入力（空欄）のエントリは警告だけ出してスキップする。
	valid := make([]ResponseEntry, 0, len(entries))
	skipped := 0
	for _, e := range entries {
		if strings.TrimSpace(e.Reply) == "" {
			skipped++
			continue
		}
		valid = append(valid, e)
	}
	if skipped > 0 {
		fmt.Printf("[情報] reply が未入力のため %d 件のエントリをスキップしました\n", skipped)
	}

	return valid, nil
}

// similarity は2つの文字列の類似度を 0.0〜1.0 で返す。
// Python版で使っていた difflib.SequenceMatcher の簡易代用として、
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
// 最良の応答を選ぶ。どの候補も閾値未満ならfallbackReplyを返す。
func findBestResponse(
	input string,
	entries []ResponseEntry,
	bm25Index *BM25Index,
	reranker CrossEncoderScorer,
) (reply string, score float64) {
	// 第1相: BM25で候補を topK 件に絞り込む
	candidates := bm25Index.Search(input, bm25TopK)
	if len(candidates) == 0 {
		return fallbackReply, 0
	}

	maxBM25 := candidates[0].Score // Search はスコア降順で返す

	// 第2相: 絞り込んだ候補だけをクエリとのペアで精査し、並べ替える
	bestScore := 0.0
	bestReply := ""
	for _, cand := range candidates {
		entry := entries[cand.EntryIndex]
		s := reranker.Score(input, entry, cand, maxBM25)
		if s > bestScore {
			bestScore = s
			bestReply = entry.Reply
		}
	}

	if bestScore >= rerankThreshold {
		return bestReply, bestScore
	}
	return fallbackReply, bestScore
}

// audioQuery / synthesis の最低限のレスポンス型（中身は素通しでよいので生JSONのまま扱う）

func speak(text string, speaker int, outPath string) error {
	client := &http.Client{Timeout: 10 * time.Second}

	// 1. audio_query
	queryParams := url.Values{}
	queryParams.Set("text", text)
	queryParams.Set("speaker", fmt.Sprintf("%d", speaker))
	queryURL := fmt.Sprintf("%s/audio_query?%s", voicevoxBaseURL, queryParams.Encode())

	queryReq, err := http.NewRequest(http.MethodPost, queryURL, nil)
	if err != nil {
		return err
	}
	queryResp, err := client.Do(queryReq)
	if err != nil {
		return fmt.Errorf("VOICEVOXサーバーに接続できませんでした: %w", err)
	}
	defer queryResp.Body.Close()

	if queryResp.StatusCode != http.StatusOK {
		var errBody bytes.Buffer
		errBody.ReadFrom(queryResp.Body)
		return fmt.Errorf("audio_query が失敗しました（status: %d, body: %s）", queryResp.StatusCode, errBody.String())
	}

	var queryBody bytes.Buffer
	if _, err := queryBody.ReadFrom(queryResp.Body); err != nil {
		return err
	}

	// 2. synthesis
	synthURL := fmt.Sprintf("%s/synthesis?speaker=%d", voicevoxBaseURL, speaker)
	synthReq, err := http.NewRequest(http.MethodPost, synthURL, bytes.NewReader(queryBody.Bytes()))
	if err != nil {
		return err
	}
	synthReq.Header.Set("Content-Type", "application/json")

	synthResp, err := client.Do(synthReq)
	if err != nil {
		return fmt.Errorf("VOICEVOXサーバーに接続できませんでした: %w", err)
	}
	defer synthResp.Body.Close()

	if synthResp.StatusCode != http.StatusOK {
		var errBody bytes.Buffer
		errBody.ReadFrom(synthResp.Body)
		return fmt.Errorf("synthesis が失敗しました（status: %d, body: %s）", synthResp.StatusCode, errBody.String())
	}

	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.ReadFrom(synthResp.Body); err != nil {
		return err
	}

	fmt.Printf("[音声を %s に保存しました]\n", outPath)
	return nil
}

func main() {
	entries, err := loadResponses(responsesFilePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "エラー:", err)
		os.Exit(1)
	}

	if len(entries) == 0 {
		fmt.Println("[警告] 有効な応答（replyが入力済みのもの）がありません。responses.json を編集してください。")
	}

	bm25Index := buildBM25Index(entries)
	reranker := LexicalCrossEncoder{}

	fmt.Println("=== 話し相手プログラム（Go版）===")
	fmt.Println("終了するには exit と入力してください")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("あなた: ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "exit" || input == "quit" || input == "終了" {
			fmt.Println("相手: またね！")
			break
		}
		if input == "" {
			continue
		}

		reply, _ := findBestResponse(input, entries, bm25Index, reranker)
		fmt.Printf("相手: %s\n", reply)

		if err := speak(reply, defaultSpeakerID, defaultOutputWavPath); err != nil {
			fmt.Printf("[音声生成に失敗しました: %v]\n", err)
			fmt.Println("  → VOICEVOXが起動していない場合は https://voicevox.hiroshiba.jp からダウンロードしてください")
		}
	}
}
