package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"ai-arena/internal/auth"
	"ai-arena/internal/broker"
	"ai-arena/internal/runtimeguard"
	"ai-arena/internal/tokenledger"
	"ai-arena/internal/worldstate"
)

func main() {
	mode := flag.String("mode", "demo", "Mode: demo|status|quota|recover|reset|admit|binding|self-status|self-reboot|self-snapshot|self-restore|self-request-cpu|self-request-memory|self-request-disk|self-request-gpu-time|self-request-vps-access|self-submit-result|get-thread|messages|thread-summary|host-inbox|host-followups|reply|ignore|tickets|ticket|get-ticket|ticket-reply")
	residentID := flag.String("resident", "jade", "Resident ID")
	hours := flag.Float64("hours", 1, "Recovery hours to advance for recover mode")
	recoveryMode := flag.String("recovery-mode", "", "Optional recovery mode for recover mode: idle|normal|rest|deep")
	kind := flag.String("kind", "work", "Call kind for admit mode: work|final_notice")
	apply := flag.Bool("apply", false, "Whether admit mode should actually apply the call")
	model := flag.String("model", "", "Optional model override for admit mode")
	inputTokens := flag.Int("input-tokens", -1, "Optional input tokens for admit mode")
	cachedTokens := flag.Int("cached-tokens", -1, "Optional cached input tokens for admit mode")
	outputTokens := flag.Int("output-tokens", -1, "Optional output tokens for admit mode")
	totalTokens := flag.Int("total-tokens", -1, "Optional total tokens for admit mode")
	toolCalls := flag.Int("tool-calls", -1, "Optional tool call count penalty for admit mode")
	responseID := flag.String("response-id", "", "Optional response id for admit mode")
	limit := flag.Int("limit", 8, "Message limit for messages mode")
	status := flag.String("status", "", "Optional status filter for messages mode")
	messageID := flag.String("message-id", "", "Target message ID for reply/ignore modes")
	body := flag.String("body", "", "Reply body for reply mode")
	priority := flag.String("priority", "", "Optional ticket priority filter or value: low|medium|high|urgent")
	title := flag.String("title", "", "Optional title for ticket modes that create one")
	amount := flag.String("amount", "", "Requested amount for self-request-* modes")
	reason := flag.String("reason", "", "Request reason for self-request-* modes")
	summary := flag.String("summary", "", "Summary for self-submit-result")
	snapshotName := flag.String("snapshot-name", "", "Snapshot name for self-snapshot/self-restore")
	closeTicket := flag.Bool("close-ticket", false, "Whether ticket-reply should close the ticket")
	flag.Parse()

	app := broker.New(".agents")
	world := worldstate.New(".agents")
	host := broker.NewHostActionService(".agents")

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
	case "quota":
		out, err := app.RunQuota(*residentID)
		if err != nil {
			exitf("%v", err)
		}
		printJSON(out)
	case "recover":
		now := time.Now().UTC()
		var (
			out any
			err error
		)
		if *recoveryMode != "" {
			out, err = app.RunRecoverWithMode(*residentID, *hours, now, *recoveryMode)
		} else {
			out, err = app.RunRecover(*residentID, *hours, now)
		}
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
	case "binding":
		out, ok := app.Binding(*residentID)
		if !ok {
			exitf("unknown resident binding: %s", *residentID)
		}
		printJSON(out)
	case "self-status":
		out, err := broker.NewSelfService(app).Status(auth.ResidentClaim{ResidentID: *residentID})
		if err != nil {
			exitf("%v", err)
		}
		printJSON(out)
	case "self-reboot":
		out, err := broker.NewSelfService(app).RequestReboot(auth.ResidentClaim{ResidentID: *residentID})
		if err != nil {
			exitf("%v", err)
		}
		printJSON(out)
	case "self-snapshot":
		out, err := broker.NewSelfService(app).RequestSnapshot(auth.ResidentClaim{ResidentID: *residentID}, *snapshotName)
		if err != nil {
			exitf("%v", err)
		}
		printJSON(out)
	case "self-restore":
		out, err := broker.NewSelfService(app).RequestRestore(auth.ResidentClaim{ResidentID: *residentID}, *snapshotName)
		if err != nil {
			exitf("%v", err)
		}
		printJSON(out)
	case "self-request-cpu":
		out, err := broker.NewSelfService(app).RequestCPU(auth.ResidentClaim{ResidentID: *residentID}, broker.ResourceRequestInput{
			Amount:  *amount,
			Reason:  *reason,
			Urgency: *priority,
		})
		if err != nil {
			exitf("%v", err)
		}
		printJSON(out)
	case "self-request-memory":
		out, err := broker.NewSelfService(app).RequestMemory(auth.ResidentClaim{ResidentID: *residentID}, broker.ResourceRequestInput{
			Amount:  *amount,
			Reason:  *reason,
			Urgency: *priority,
		})
		if err != nil {
			exitf("%v", err)
		}
		printJSON(out)
	case "self-request-disk":
		out, err := broker.NewSelfService(app).RequestDisk(auth.ResidentClaim{ResidentID: *residentID}, broker.ResourceRequestInput{
			Amount:  *amount,
			Reason:  *reason,
			Urgency: *priority,
		})
		if err != nil {
			exitf("%v", err)
		}
		printJSON(out)
	case "self-request-gpu-time":
		out, err := broker.NewSelfService(app).RequestGPUTime(auth.ResidentClaim{ResidentID: *residentID}, broker.ResourceRequestInput{
			Amount:  *amount,
			Reason:  *reason,
			Urgency: *priority,
		})
		if err != nil {
			exitf("%v", err)
		}
		printJSON(out)
	case "self-request-vps-access":
		out, err := broker.NewSelfService(app).RequestVPSAccess(auth.ResidentClaim{ResidentID: *residentID}, broker.ResourceRequestInput{
			Amount:  *amount,
			Reason:  *reason,
			Urgency: *priority,
		})
		if err != nil {
			exitf("%v", err)
		}
		printJSON(out)
	case "self-submit-result":
		out, err := broker.NewSelfService(app).SubmitResult(auth.ResidentClaim{ResidentID: *residentID}, broker.SubmissionInput{
			Title:   *title,
			Summary: *summary,
			Details: *body,
		})
		if err != nil {
			exitf("%v", err)
		}
		printJSON(out)
	case "messages":
		out, err := world.ReadMessagesByStatus(*residentID, *status, *limit)
		if err != nil {
			exitf("%v", err)
		}
		printJSON(out)
	case "get-thread":
		out, err := world.ReadMessagesByStatus(*residentID, *status, *limit)
		if err != nil {
			exitf("%v", err)
		}
		printJSON(out)
	case "thread-summary":
		out, err := world.ReadAllThreadSummaries()
		if err != nil {
			exitf("%v", err)
		}
		printJSON(out)
	case "host-inbox":
		out, err := world.ReadHostInboxSummary(*limit, *limit)
		if err != nil {
			exitf("%v", err)
		}
		printJSON(out)
	case "host-followups":
		out, err := world.ReadHostFollowups(*limit)
		if err != nil {
			exitf("%v", err)
		}
		printJSON(out)
	case "reply":
		if *messageID == "" {
			exitf("message-id is required for reply mode")
		}
		if err := worldstate.ValidateReplyBody(*body); err != nil {
			exitf("%v", err)
		}
		out, err := host.Reply(*messageID, *body)
		if err != nil {
			exitf("%v", err)
		}
		printJSON(out)
	case "ignore":
		if *messageID == "" {
			exitf("message-id is required for ignore mode")
		}
		out, err := host.Ignore(*messageID)
		if err != nil {
			exitf("%v", err)
		}
		printJSON(out)
	case "tickets":
		out, err := world.ReadTickets(*residentID, *status, *priority, *limit)
		if err != nil {
			exitf("%v", err)
		}
		printJSON(out)
	case "get-ticket":
		if *messageID == "" {
			exitf("message-id is required for get-ticket mode")
		}
		out, err := world.ReadTicket(*messageID)
		if err != nil {
			exitf("%v", err)
		}
		printJSON(out)
	case "ticket":
		if *residentID == "" {
			exitf("resident is required for ticket mode")
		}
		if *title == "" {
			exitf("title is required for ticket mode")
		}
		out, err := world.CreateResidentTicket(*residentID, *title, *body, *priority, time.Now().UTC())
		if err != nil {
			exitf("%v", err)
		}
		printJSON(out)
	case "ticket-reply":
		if *messageID == "" {
			exitf("message-id is required for ticket-reply mode")
		}
		out, err := host.ReplyTicket(*messageID, *body, *closeTicket)
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
