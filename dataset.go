// dataset.go
//
// 応答データセット（responses.json）の型と読み込み処理。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const responsesFilePath = "responses.json"

// validExpressions はGUIの立ち絵が対応している表情の一覧。
// ここにない値、または空欄は "normal" 扱いにする。
var validExpressions = map[string]bool{
	"normal": true,
	"happy":  true,
	"sad":    true,
	"tired":  true,
}

// ResponseEntry は応答データセット1件分の構造。
// triggers には言い回しのバリエーションを複数入れられる。
type ResponseEntry struct {
	ID         string   `json:"id"`
	Triggers   []string `json:"triggers"`
	Note       string   `json:"note"`
	Reply      string   `json:"reply"`
	Expression string   `json:"expression"` // normal / happy / sad / tired
}

// NormalizedExpression は未知の値や空欄を "normal" に丸めて返す。
func (e ResponseEntry) NormalizedExpression() string {
	if validExpressions[e.Expression] {
		return e.Expression
	}
	return "normal"
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
