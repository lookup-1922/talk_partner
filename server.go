// server.go
//
// GUIモード: ローカルにHTTPサーバーを立て、既定のブラウザで開く。
// ネイティブGUIツールキット（CGOが必要になりがち）を避け、
// 標準ライブラリだけで完結させるための構成。
package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os/exec"
	"runtime"
)

//go:embed web
var webFiles embed.FS

const guiPort = "8765"

type server struct {
	entries   []ResponseEntry
	bm25Index *BM25Index
	reranker  CrossEncoderScorer
}

type chatRequest struct {
	Message string `json:"message"`
}

type speakRequest struct {
	Text string `json:"text"`
}

func (s *server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	result := findBestResponse(req.Message, s.entries, s.bm25Index, s.reranker)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *server) handleSpeak(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req speakRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	audio, err := synthesizeAudio(req.Text, defaultSpeakerID)
	if err != nil {
		// VOICEVOXが起動していない/失敗した場合は502で返す。
		// フロントエンド側はこれを検知して音声なしで続行する。
		log.Printf("[speak] %v", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "audio/wav")
	w.Write(audio)
}

// runGUI はHTTPサーバーを起動し、既定のブラウザで開く。
func runGUI(entries []ResponseEntry, bm25Index *BM25Index, reranker CrossEncoderScorer) {
	if len(entries) == 0 {
		fmt.Println("[警告] 有効な応答（replyが入力済みのもの）がありません。responses.json を編集してください。")
	}

	s := &server{entries: entries, bm25Index: bm25Index, reranker: reranker}

	webRoot, err := fs.Sub(webFiles, "web")
	if err != nil {
		log.Fatalf("静的ファイルの読み込みに失敗しました: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(webRoot)))
	mux.HandleFunc("/api/chat", s.handleChat)
	mux.HandleFunc("/api/speak", s.handleSpeak)

	url := fmt.Sprintf("http://localhost:%s", guiPort)
	fmt.Printf("=== 話し相手プログラム（GUIモード）===\n")
	fmt.Printf("ブラウザで %s を開きます（自動で開かない場合は手動でアクセスしてください）\n", url)
	fmt.Println("終了するにはこのウィンドウで Ctrl+C を押してください")

	go openBrowser(url)

	if err := http.ListenAndServe("127.0.0.1:"+guiPort, mux); err != nil {
		log.Fatalf("サーバーの起動に失敗しました: %v", err)
	}
}

// openBrowser はOSごとの方法で既定のブラウザにURLを開かせる。
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default: // linux, etc.
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		fmt.Printf("[情報] ブラウザを自動で開けませんでした。手動で %s を開いてください\n", url)
	}
}
