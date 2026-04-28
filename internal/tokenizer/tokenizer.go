// Package tokenizer 提供基于 tiktoken 的精确 token 计数。
package tokenizer

import (
	"bufio"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/pkoukk/tiktoken-go"

	"anthropic-openai-gateway/pkg/types"
)

//go:embed assets/*.tiktoken
var embeddedBPEFiles embed.FS

var (
	encMu    sync.Mutex
	encCache = map[string]*tiktoken.Tiktoken{}
)

func init() {
	tiktoken.SetBpeLoader(embeddedBPELoader{})
}

// CountTokens 计算 Anthropic MessageRequest 的 input_tokens。
// 使用 tiktoken 精确编码；tiktoken 不可用时返回 -1。
// 模型识别规则：
//   - o200k_base: gpt-4o*, gpt-4.1*, gpt-4.5*, gpt-5, o3*, o4*, deepseek-v4*
//   - cl100k_base: 其余所有
func CountTokens(req *types.MessageRequest) int {
	if req == nil {
		return -1
	}
	enc := encodingForModel(req.Model)
	if enc == nil {
		return -1
	}

	total := 0
	total += countContent(enc, req.System)
	for _, msg := range req.Messages {
		total += countMessage(enc, msg)
	}
	for _, tool := range req.Tools {
		total += countTool(enc, tool)
	}
	if total == 0 {
		return 1
	}
	return total
}

// encodingForModel 返回模型对应的 tiktoken encoding。初始化失败时返回 nil。
func encodingForModel(model string) *tiktoken.Tiktoken {
	encName := resolveEncodingName(model)
	encMu.Lock()
	defer encMu.Unlock()
	if cached, ok := encCache[encName]; ok {
		return cached
	}
	enc, err := tiktoken.GetEncoding(encName)
	if err != nil {
		return nil
	}
	encCache[encName] = enc
	return enc
}

// resolveEncodingName 根据模型名前缀返回 encoding 名称。
func resolveEncodingName(model string) string {
	prefixes := []string{"gpt-4o", "gpt-4.1", "gpt-4.5", "gpt-5", "o3", "o4", "deepseek-v4"}
	for _, p := range prefixes {
		if len(model) >= len(p) && model[:len(p)] == p {
			return "o200k_base"
		}
	}
	return "cl100k_base"
}

type embeddedBPELoader struct{}

func (embeddedBPELoader) LoadTiktokenBpe(tiktokenBpeFile string) (map[string]int, error) {
	name := path.Base(tiktokenBpeFile)
	switch name {
	case "o200k_base.tiktoken", "cl100k_base.tiktoken":
	default:
		return nil, fmt.Errorf("unsupported embedded tiktoken BPE: %s", tiktokenBpeFile)
	}

	contents, err := embeddedBPEFiles.ReadFile("assets/" + name)
	if err != nil {
		return nil, fmt.Errorf("read embedded tiktoken BPE %s: %w", name, err)
	}
	return parseTiktokenBPE(contents)
}

func parseTiktokenBPE(contents []byte) (map[string]int, error) {
	ranks := make(map[string]int)
	scanner := bufio.NewScanner(strings.NewReader(string(contents)))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Split(line, " ")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid tiktoken BPE line: %q", line)
		}
		token, err := base64.StdEncoding.DecodeString(parts[0])
		if err != nil {
			return nil, fmt.Errorf("decode token %q: %w", parts[0], err)
		}
		rank, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("parse rank %q: %w", parts[1], err)
		}
		ranks[string(token)] = rank
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan tiktoken BPE: %w", err)
	}
	return ranks, nil
}

// countContent 对 system 等 interface{} 内容进行编码计数。
// 支持 string 和 []ContentBlock（JSON 反序列化后为 []interface{}{map[string]interface{}}）。
func countContent(enc *tiktoken.Tiktoken, content interface{}) int {
	switch v := content.(type) {
	case string:
		return len(enc.Encode(v, nil, nil))
	case []interface{}:
		total := 0
		for _, item := range v {
			if block, ok := item.(map[string]interface{}); ok {
				for _, key := range []string{"text", "thinking"} {
					if text, ok := block[key].(string); ok {
						total += len(enc.Encode(text, nil, nil))
					}
				}
				// tool_result 嵌套 content
				if nested, ok := block["content"]; ok {
					total += countContent(enc, nested)
				}
			}
		}
		return total
	}
	return 0
}

// countMessage 编码单条消息（role + content + 格式开销）。
func countMessage(enc *tiktoken.Tiktoken, msg types.Message) int {
	total := len(enc.Encode(msg.Role, nil, nil))
	total += countContent(enc, msg.Content)
	total += 4 // 消息格式开销（OpenAI 消息分隔符）
	return total
}

// countTool 编码单个工具定义（name + description + schema + 格式开销）。
func countTool(enc *tiktoken.Tiktoken, tool types.Tool) int {
	total := len(enc.Encode(tool.Name, nil, nil))
	total += len(enc.Encode(tool.Description, nil, nil))
	if schemaBytes, err := json.Marshal(tool.InputSchema); err == nil {
		total += len(enc.Encode(string(schemaBytes), nil, nil))
	}
	total += 4 // 工具格式开销
	return total
}
