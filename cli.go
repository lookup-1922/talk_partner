// cli.go
//
// CLIモード（従来のターミナル対話）。GUIが使えない環境向けに残してある。
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

const defaultOutputWavPath = "output.wav"

func runCLI(entries []ResponseEntry, bm25Index *BM25Index, reranker CrossEncoderScorer) {
	fmt.Println("=== 話し相手プログラム（CLIモード）===")
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

		result := findBestResponse(input, entries, bm25Index, reranker)
		fmt.Printf("相手: %s\n", result.Reply)

		if err := speak(result.Reply, defaultSpeakerID, defaultOutputWavPath); err != nil {
			fmt.Printf("[音声生成に失敗しました: %v]\n", err)
			fmt.Println("  → VOICEVOXが起動していない場合は https://voicevox.hiroshiba.jp からダウンロードしてください")
		}
	}
}
