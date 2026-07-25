package llm

import (
	"fudge/rules"

	"github.com/memmaker/gollum/llm"
)

// ExecuteToolCalls processes a slice of tool calls, executes them, and returns
// corresponding tool result messages to append to the conversation history.
func ExecuteToolCalls(w *rules.World, calls []llm.ToolCall, debugFn func(llm.ToolCall, string, error)) []llm.Message {
	var results []llm.Message

	for _, toolCall := range calls {
		result, err := executeAction(w, toolCall)
		debugFn(toolCall, result, err)
		if err != nil {
			result = "Error: " + err.Error()
		}
		results = append(results, llm.Message{
			Role:       llm.RoleTool,
			ToolCallID: toolCall.ID,
			Name:       toolCall.Function.Name,
			Content:    result,
		})
	}

	return results
}

// executeAction is defined in tools.go and called here
// We import it from the main package via a function pointer
var executeAction func(*rules.World, llm.ToolCall) (string, error)

func SetExecuteAction(fn func(*rules.World, llm.ToolCall) (string, error)) {
	executeAction = fn
}
