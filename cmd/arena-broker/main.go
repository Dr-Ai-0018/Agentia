package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"ai-arena/internal/broker"
	"ai-arena/internal/runtimeguard"
)

func main() {
	mode := flag.String("mode", "demo", "Mode: demo|status|recover|reset|admit")
	residentID := flag.String("resident", "jade", "Resident ID")
	hours := flag.Float64("hours", 1, "Recovery hours to advance for recover mode")
	kind := flag.String("kind", "work", "Call kind for admit mode: work|final_notice")
	apply := flag.Bool("apply", false, "Whether admit mode should actually apply the call")
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
		out, err := app.RunAdmit(*residentID, callKind, *apply, time.Now().UTC())
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
