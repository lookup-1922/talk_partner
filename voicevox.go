// voicevox.go
//
// VOICEVOXのローカルサーバー(localhost:50021)と通信し、音声を合成する。
package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"
)

const (
	voicevoxBaseURL  = "http://localhost:50021"
	defaultSpeakerID = 108 // 東北きりたん
)

// synthesizeAudio はVOICEVOXにテキストを送り、生成されたwavのバイト列を返す。
// 流れ: audio_query でクエリ生成 → synthesis で音声合成。
func synthesizeAudio(text string, speaker int) ([]byte, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	// 1. audio_query
	queryParams := url.Values{}
	queryParams.Set("text", text)
	queryParams.Set("speaker", fmt.Sprintf("%d", speaker))
	queryURL := fmt.Sprintf("%s/audio_query?%s", voicevoxBaseURL, queryParams.Encode())

	queryReq, err := http.NewRequest(http.MethodPost, queryURL, nil)
	if err != nil {
		return nil, err
	}
	queryResp, err := client.Do(queryReq)
	if err != nil {
		return nil, fmt.Errorf("VOICEVOXサーバーに接続できませんでした: %w", err)
	}
	defer queryResp.Body.Close()

	if queryResp.StatusCode != http.StatusOK {
		var errBody bytes.Buffer
		errBody.ReadFrom(queryResp.Body)
		return nil, fmt.Errorf("audio_query が失敗しました（status: %d, body: %s）", queryResp.StatusCode, errBody.String())
	}

	var queryBody bytes.Buffer
	if _, err := queryBody.ReadFrom(queryResp.Body); err != nil {
		return nil, err
	}

	// 2. synthesis
	synthURL := fmt.Sprintf("%s/synthesis?speaker=%d", voicevoxBaseURL, speaker)
	synthReq, err := http.NewRequest(http.MethodPost, synthURL, bytes.NewReader(queryBody.Bytes()))
	if err != nil {
		return nil, err
	}
	synthReq.Header.Set("Content-Type", "application/json")

	synthResp, err := client.Do(synthReq)
	if err != nil {
		return nil, fmt.Errorf("VOICEVOXサーバーに接続できませんでした: %w", err)
	}
	defer synthResp.Body.Close()

	if synthResp.StatusCode != http.StatusOK {
		var errBody bytes.Buffer
		errBody.ReadFrom(synthResp.Body)
		return nil, fmt.Errorf("synthesis が失敗しました（status: %d, body: %s）", synthResp.StatusCode, errBody.String())
	}

	var audioBody bytes.Buffer
	if _, err := audioBody.ReadFrom(synthResp.Body); err != nil {
		return nil, err
	}
	return audioBody.Bytes(), nil
}

// speak はCLIモード用に、合成した音声をファイルへ保存する。
func speak(text string, speaker int, outPath string) error {
	data, err := synthesizeAudio(text, speaker)
	if err != nil {
		return err
	}
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		return err
	}
	fmt.Printf("[音声を %s に保存しました]\n", outPath)
	return nil
}
