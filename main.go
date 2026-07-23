package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"fudge/rules"

	"github.com/memmaker/gollum/llm"
	"golang.org/x/term"
)

// Minimal in-process interactive shell with prose-driven flow using 4dF checks.
func main() {
	w, err := rules.NewWorld(987654321, "rulesets/example")
	if err != nil {
		fmt.Println("failed to load ruleset:", err)
		return
	}
	hero := w.CreateCharacter(
		"pc1",
		"Hero",
		map[string]int{"strength": 1, "agility": 1, "perception": 0},
		map[string]int{"athletics": 2, "stealth": 1, "fight": 1},
	)
	goblin := w.CreateCharacter(
		"gob1",
		"Goblin",
		map[string]int{"strength": 0, "agility": 1, "perception": 0},
		map[string]int{"athletics": 1, "stealth": 0, "fight": 1},
	)
	narration := []string{"You awaken in a dim dungeon. A goblin blocks your path.", "Tell me what you do."}

	reader := bufio.NewReader(os.Stdin)
	for {
		// Render narration
		printNarration(narration)

		// Player prompt
		fmt.Print("You: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading input; exiting.")
			return
		}
		line = strings.TrimSpace(line)
		if line == "quit" || line == "exit" {
			fmt.Println("Session ended.")
			return
		}

		if line == "" {
			continue
		}

		text, err := narrateAction(w, hero.ID, goblin.ID, line)
		if err != nil {
			fmt.Println("LLM error:", err)
			continue
		}
		narration = []string{text}
	}
}

const dmPrompt = `You are the dungeon master for a Fudge RPG. Trivial character actions don't need any skill checks. If the player's action warrants a skill check (action needing some skill, complex action, uncertain outcomes, opposed actions), call exactly one available tool to resolve it. Then narrate the outcome briefly using the tool result. Do not use dice or randomness yourself.`

const llmRequestTimeout = time.Minute

func narrateAction(w *rules.World, heroID, goblinID, prompt string) (string, error) {
	client, model, _, err := llm.InitClientFromHome()
	if err != nil {
		return "", err
	}

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: dmPrompt},
		{Role: llm.RoleUser, Content: prompt},
	}
	response, err := chatWithTimeout(client, model, messages, actionTools())
	if err != nil {
		return "", err
	}
	if len(response.Message.ToolCalls) == 0 {
		if response.Message.Content == "" {
			return "", fmt.Errorf("LLM returned neither narration nor an action tool call")
		}
		return response.Message.Content, nil
	}
	if len(response.Message.ToolCalls) != 1 {
		return "", fmt.Errorf("expected one action tool call, got %d", len(response.Message.ToolCalls))
	}

	messages = append(messages, response.Message)
	result, err := executeAction(w, heroID, goblinID, response.Message.ToolCalls[0])
	if err != nil {
		return "", err
	}
	messages = append(messages, llm.Message{
		Role:       llm.RoleTool,
		ToolCallID: response.Message.ToolCalls[0].ID,
		Name:       response.Message.ToolCalls[0].Function.Name,
		Content:    result,
	})
	response, err = chatWithTimeout(client, model, messages, nil)
	if err != nil {
		return "", err
	}
	if response.Message.Content == "" {
		return "", fmt.Errorf("LLM returned no narration")
	}
	return response.Message.Content, nil
}

func chatWithTimeout(client *llm.Client, model string, messages []llm.Message, tools []llm.Tool) (llm.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), llmRequestTimeout)
	defer cancel()
	return client.Chat(ctx, llm.Request{Model: model, Messages: messages, Tools: tools})
}

func actionTools() []llm.Tool {
	return []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{Name: "query_ruleset", Description: "Query available attributes, skills, species, gifts, and faults in the current ruleset.", Parameters: map[string]any{"type": "object", "properties": map[string]any{}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "unopposed_skill_check", Description: "Resolve a skill check against a difficulty.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"skill": map[string]any{"type": "string"}, "difficulty": map[string]any{"type": "integer"}}, "required": []string{"skill", "difficulty"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "opposed_skill_check", Description: "Resolve a skill check against an opponent.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"actorSkill": map[string]any{"type": "string"}, "oppID": map[string]any{"type": "string"}, "oppSkill": map[string]any{"type": "string"}}, "required": []string{"actorSkill", "oppSkill"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "write_ruleset_file", Description: "Write one recognized ruleset file in replace or append mode.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"filename": map[string]any{"type": "string"}, "mode": map[string]any{"type": "string", "enum": []string{"replace", "append"}}, "content": map[string]any{"type": "string"}}, "required": []string{"filename", "mode", "content"}}}},
		{Type: "function", Function: llm.ToolFunction{Name: "create_character", Description: "Create a new character in the selected ruleset.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"slug": map[string]any{"type": "string"}, "name": map[string]any{"type": "string"}, "species": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"}, "location": map[string]any{"type": "string"}, "attributes": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}}, "skills": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}}, "gifts": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "faults": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}, "required": []string{"name", "species"}}}},
	}
}

func queryRuleset(w *rules.World) (string, error) {
	if w.Ruleset == nil {
		return "", fmt.Errorf("world has no selected ruleset")
	}

	var sb strings.Builder
	sb.WriteString("Available traits in ruleset:\n\n")

	sb.WriteString("ATTRIBUTES:\n")
	for _, attr := range sortedAttributeKeys(w.Ruleset.Attributes) {
		desc := w.Ruleset.Attributes[attr].Description
		sb.WriteString(fmt.Sprintf("  - %s: %s\n", attr, desc))
	}

	sb.WriteString("\nSKILLS:\n")
	for _, skill := range sortedSkillKeys(w.Ruleset.Skills) {
		desc := w.Ruleset.Skills[skill].Description
		sb.WriteString(fmt.Sprintf("  - %s: %s\n", skill, desc))
	}

	sb.WriteString("\nSPECIES:\n")
	for _, spec := range sortedSpeciesKeys(w.Ruleset.Species) {
		desc := w.Ruleset.Species[spec].Description
		scale := w.Ruleset.Species[spec].Scale
		sb.WriteString(fmt.Sprintf("  - %s (scale %d): %s\n", spec, scale, desc))
	}

	sb.WriteString("\nGIFTS:\n")
	for _, gift := range sortedGiftKeys(w.Ruleset.Gifts) {
		desc := w.Ruleset.Gifts[gift].Description
		sb.WriteString(fmt.Sprintf("  - %s: %s\n", gift, desc))
	}

	sb.WriteString("\nFAULTS:\n")
	for _, fault := range sortedFaultKeys(w.Ruleset.Faults) {
		desc := w.Ruleset.Faults[fault].Description
		sb.WriteString(fmt.Sprintf("  - %s: %s\n", fault, desc))
	}

	return sb.String(), nil
}

func sortedAttributeKeys(m map[string]*rules.Attribute) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedSkillKeys(m map[string]*rules.Skill) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedSpeciesKeys(m map[string]*rules.Species) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedGiftKeys(m map[string]*rules.Gift) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedFaultKeys(m map[string]*rules.Fault) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func executeAction(w *rules.World, heroID, goblinID string, call llm.ToolCall) (string, error) {
	switch call.Function.Name {
	case "query_ruleset":
		return queryRuleset(w)
	case "unopposed_skill_check":
		var args struct {
			Skill      string `json:"skill"`
			Difficulty int    `json:"difficulty"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse unopposed skill check: %w", err)
		}
		if args.Skill == "" {
			return "", fmt.Errorf("unopposed skill check requires skill")
		}
		success, total, roll, err := w.UnopposedSkillCheck(heroID, args.Skill, args.Difficulty)
		if err != nil {
			return "", err
		}
		hero, _ := w.Character(heroID)
		return fmt.Sprintf("%s rolled %d + %d = %d against %d: success=%t", hero.Name, roll, hero.Skills[strings.ToLower(args.Skill)], total, args.Difficulty, success), nil
	case "opposed_skill_check":
		var args struct {
			ActorSkill string `json:"actorSkill"`
			OppID      string `json:"oppID"`
			OppSkill   string `json:"oppSkill"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse opposed skill check: %w", err)
		}
		if args.ActorSkill == "" || args.OppSkill == "" {
			return "", fmt.Errorf("opposed skill check requires actorSkill and oppSkill")
		}
		if args.OppID == "" {
			args.OppID = goblinID
		}
		wins, actorTotal, opponentTotal, margin, err := w.OpposedSkillCheck(heroID, args.ActorSkill, args.OppID, args.OppSkill)
		if err != nil {
			return "", err
		}
		hero, _ := w.Character(heroID)
		opponent, _ := w.Character(args.OppID)
		return fmt.Sprintf("%s scored %d against %s's %d (relative degree %d): success=%t", hero.Name, actorTotal, opponent.Name, opponentTotal, margin, wins), nil
	case "write_ruleset_file":
		var args struct {
			Filename string `json:"filename"`
			Mode     string `json:"mode"`
			Content  string `json:"content"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("parse write ruleset file: %w", err)
		}
		if w.Ruleset == nil {
			return "", fmt.Errorf("world has no ruleset selected")
		}
		if args.Filename == "" || args.Mode == "" {
			return "", fmt.Errorf("write ruleset file requires filename, mode, and content")
		}
		if err := w.Ruleset.WriteFile(args.Filename, args.Mode, []byte(args.Content)); err != nil {
			return "", err
		}
		return fmt.Sprintf("wrote %s (%s)", args.Filename, args.Mode), nil
	case "create_character":
		var req struct {
			Slug        string         `json:"slug"`
			Name        string         `json:"name"`
			Species     string         `json:"species"`
			Description string         `json:"description"`
			Location    string         `json:"location"`
			Attributes  map[string]int `json:"attributes"`
			Skills      map[string]int `json:"skills"`
			Gifts       []string       `json:"gifts"`
			Faults      []string       `json:"faults"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &req); err != nil {
			return "", fmt.Errorf("parse create_character: %w", err)
		}
		ch, err := w.CreateCharacterValidated(rules.CharacterRequest(req))
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("created character %s", ch.ID), nil
	default:
		return "", fmt.Errorf("unknown action tool %q", call.Function.Name)
	}
}

func printNarration(log []string) {
	const maxLines = 12
	start := 0
	if len(log) > maxLines {
		start = len(log) - maxLines
	}
	fmt.Println("--- DM Narration ---")
	for i := start; i < len(log); i++ {
		fmt.Println(wrapText(log[i], terminalWidth()))
	}
	fmt.Println("--------------------")
}

func terminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 {
		return 80
	}
	return width
}

func wrapText(text string, width int) string {
	var wrapped []string
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			wrapped = append(wrapped, "")
			continue
		}
		wrapped = append(wrapped, wrapLine(line, width)...)
	}
	return strings.Join(wrapped, "\n")
}

func wrapLine(line string, width int) []string {
	if width <= 0 {
		return []string{line}
	}

	var wrapped []string
	var current string
	for _, word := range strings.Fields(line) {
		for len([]rune(word)) > width {
			if current != "" {
				wrapped = append(wrapped, current)
				current = ""
			}
			runes := []rune(word)
			wrapped = append(wrapped, string(runes[:width]))
			word = string(runes[width:])
		}
		if current == "" {
			current = word
			continue
		}
		if len([]rune(current))+1+len([]rune(word)) <= width {
			current += " " + word
			continue
		}
		wrapped = append(wrapped, current)
		current = word
	}
	if current != "" {
		wrapped = append(wrapped, current)
	}
	return wrapped
}
