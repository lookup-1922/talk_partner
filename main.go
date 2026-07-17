// 話し相手プログラム（Go版）
//
// 部報80号「話し相手をプログラミング！」の設計をベースに実装。
//   - 応答生成: 事前に用意した応答セット（responses.json）から選択（dataset.go）
//   - マッチング: 2相式（match.go）
//                第1相: BM25（bm25.go）でtopK件まで高速に絞り込み
//                第2相: 軽量リランカー（rerank.go, クロスエンコーダ役）
//                       で候補を精査し、最良の1件を選ぶ
//   - 音声: VOICEVOXのローカルサーバー(localhost:50021)にHTTPで問い合わせる（voicevox.go）
//   - GUI: ローカルHTTPサーバー＋ブラウザ表示（server.go, web/）
//          CGO不要で go build だけで動かせることを優先した構成
//   - CLI: --cli 指定時のみ従来のターミナル対話（cli.go）
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	cliMode := flag.Bool("cli", false, "GUIを使わずターミナルで対話する")
	flag.Parse()

	entries, err := loadResponses(responsesFilePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "エラー:", err)
		os.Exit(1)
	}

	bm25Index := buildBM25Index(entries)
	reranker := LexicalCrossEncoder{}

	if *cliMode {
		runCLI(entries, bm25Index, reranker)
		return
	}
	runGUI(entries, bm25Index, reranker)
}
