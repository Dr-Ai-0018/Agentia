package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"ai-arena/internal/broker"
	"ai-arena/internal/runtimeguard"
	"ai-arena/internal/tokenledger"
)

func main() {
	mode := flag.String("mode", "demo", "Mode: demo|status|recover|reset|admit")
	residentID := flag.String("resident", "jade", "Resident ID")
	hours := flag.Float64("hours", 1, "Recovery hours to advance for recover mode")
	kind := flag.String("kind", "work", "Call kind for admit mode: work|final_notice")
	apply := flag.Bool("apply", false, "Whether admit mode should actually apply the call")
	model := flag.String("model", "", "Optional model override for admit mode")
	inputTokens := flag.Int("input-tokens", -1, "Optional input tokens for admit mode")
	cachedTokens := flag.Int("cached-tokens", -1, "Optional cached input tokens for admit mode")
	outputTokens := flag.Int("output-tokens", -1, "Optional output tokens for admit mode")
	totalTokens := flag.Int("total-tokens", -1, "Optional total tokens for admit mode")
	toolCalls := flag.Int("tool-calls", -1, "Optional tool call count penalty for admit mode")
	responseID := flag.String("response-id", "", "Optional response id for admit mode")
	flag.Parse()

	app := broker.New(".agents")

	switch *mode {
	case "demo":
		out, err := app.RunDemo(*residentID, time.Now().UTC())
		if err != nil {
			exitf("%v", err)
		}
		printJSON(out)
	case "status":
		out, err := app.RunStatus(*residentID)
		if err != nil {
			exitf("%v", err)
		}
		printJSON(out)
	case "recover":
		out, err := app.RunRecover(*residentID, *hours, time.Now().UTC())
		if err != nil {
			exitf("%v", err)
		}
		printJSON(out)
	case "reset":
		out, err := app.RunReset(*residentID, time.Now().UTC())
		if err != nil {
			exitf("%v", err)
		}
		printJSON(out)
	case "admit":
		callKind := runtimeguard.CallKind(*kind)
		if callKind != runtimeguard.CallKindWork && callKind != runtimeguard.CallKindFinalNotice {
			exitf("unknown admit kind: %s", *kind)
		}
		now := time.Now().UTC()
		out, err := runAdmit(app, *residentID, callKind, *apply, now, admitArgs{
			model:        *model,
			inputTokens:  *inputTokens,
			cachedTokens: *cachedTokens,
			outputTokens: *outputTokens,
			totalTokens:  *totalTokens,
			toolCalls:    *toolCalls,
			responseID:   *responseID,
		})
		if err != nil {
			exitf("%v", err)
		}
		printJSON(out)
	default:
		exitf("unknown mode: %s", *mode)
	}
}

func printJSON(v any) {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		exitf("marshal json: %v", err)
	}
	fmt.Println(string(raw))
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

type admitArgs struct {
	model        string
	inputTokens  int
	cachedTokens int
	outputTokens int
	totalTokens  int
	toolCalls    int
	responseID   string
}

func defaultAdmitArgs() admitArgs {
	return admitArgs{
		inputTokens:  -1,
		cachedTokens: -1,
		outputTokens: -1,
		totalTokens:  -1,
		toolCalls:    -1,
	}
}

func runAdmit(app *broker.App, residentID string, callKind runtimeguard.CallKind, apply bool, now time.Time, args admitArgs) (any, error) {
	if !args.hasCustomUsage() {
		return app.RunAdmit(residentID, callKind, apply, now)
	}

	spec, err := broker.DefaultSpecForKind(callKind, now)
	if err != nil {
		return nil, err
	}
	spec.Usage.InputTokens = 0
	spec.Usage.CachedTokens = 0
	spec.Usage.OutputTokens = 0
	spec.Usage.TotalTokens = 0
	spec.Penalties = tokenledger.Penalties{}
	if args.model != "" {
		spec.Usage.Model = args.model
	}
	if args.inputTokens >= 0 {
		spec.Usage.InputTokens = args.inputTokens
	}
	if args.cachedTokens >= 0 {
		spec.Usage.CachedTokens = args.cachedTokens
	}
	if args.outputTokens >= 0 {
		spec.Usage.OutputTokens = args.outputTokens
	}
	if args.totalTokens >= 0 {
		spec.Usage.TotalTokens = args.totalTokens
	}
	if args.responseID != "" {
		spec.Usage.ResponseID = args.responseID
	}
	if spec.Usage.CachedTokens > spec.Usage.InputTokens {
		return nil, fmt.Errorf("cached tokens cannot exceed input tokens")
	}
	if spec.Usage.TotalTokens <= 0 {
		spec.Usage.TotalTokens = spec.Usage.InputTokens + spec.Usage.OutputTokens
	}
	if args.toolCalls >= 0 {
		spec.Penalties = tokenledger.Penalties{ToolCallCount: args.toolCalls}
	}
	return app.RunAdmitSpec(residentID, spec, apply)
}

func (a admitArgs) hasCustomUsage() bool {
	return a.model != "" ||
		a.inputTokens >= 0 ||
		a.cachedTokens >= 0 ||
		a.outputTokens >= 0 ||
		a.totalTokens >= 0 ||
		a.toolCalls >= 0 ||
		a.responseID != ""
}
