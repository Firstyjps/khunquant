package tools

import (
	"context"
	"fmt"
	"os"
	"time"

	chart "github.com/wcharczuk/go-chart/v2"
	"github.com/wcharczuk/go-chart/v2/drawing"

	"github.com/cryptoquantumwave/khunquant/pkg/deltaneutral"
	"github.com/cryptoquantumwave/khunquant/pkg/media"
)

// RenderDeltaNeutralYieldChartTool renders the Yield History for a delta-neutral plan
// as a PNG chart image and sends it to the active chat channel.
type RenderDeltaNeutralYieldChartTool struct {
	store      *deltaneutral.Store
	mediaStore media.MediaStore
}

func NewRenderDeltaNeutralYieldChartTool(store *deltaneutral.Store) *RenderDeltaNeutralYieldChartTool {
	return &RenderDeltaNeutralYieldChartTool{store: store}
}

func (t *RenderDeltaNeutralYieldChartTool) Name() string { return NameRenderDeltaNeutralYieldChart }

func (t *RenderDeltaNeutralYieldChartTool) Description() string {
	return DescRenderDeltaNeutralYieldChart
}

func (t *RenderDeltaNeutralYieldChartTool) SetMediaStore(s media.MediaStore) {
	t.mediaStore = s
}

func (t *RenderDeltaNeutralYieldChartTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"plan_id": map[string]any{
				"type":        "integer",
				"description": "ID of the delta-neutral plan.",
			},
			"period": map[string]any{
				"type":        "string",
				"description": "Time range: 7d (default), 14d, 30d, 3m, 6m, or all.",
				"enum":        []string{"7d", "14d", "30d", "3m", "6m", "all"},
			},
			"columns": map[string]any{
				"type":        "array",
				"description": "Which series to plot. Defaults to all four: funding_rate, funding_apy, earn_apy, combined_apy.",
				"items": map[string]any{
					"type": "string",
					"enum": []string{"funding_rate", "funding_apy", "earn_apy", "combined_apy"},
				},
			},
		},
		"required": []string{"plan_id"},
	}
}

// yieldSeries describes one line in the chart.
type yieldSeries struct {
	key      string
	name     string
	hexColor string
	secondary bool // true = right (APY%) axis
}

var allYieldSeries = []yieldSeries{
	{"funding_rate", "Funding Rate", "6366f1", false},
	{"funding_apy", "Funding APY%", "10b981", true},
	{"earn_apy", "Earn APY%", "f59e0b", true},
	{"combined_apy", "Combined APY%", "ef4444", true},
}

func (t *RenderDeltaNeutralYieldChartTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	// --- validate inputs ---
	planIDf, _ := args["plan_id"].(float64)
	planID := int64(planIDf)
	if planID <= 0 {
		return ErrorResult("plan_id is required")
	}

	periodRaw, _ := args["period"].(string)
	since, periodLabel := parsePeriodSince(periodRaw)

	// Parse columns filter (default = all four).
	wantCols := map[string]bool{}
	if raw, ok := args["columns"].([]any); ok && len(raw) > 0 {
		for _, v := range raw {
			if s, ok := v.(string); ok {
				wantCols[s] = true
			}
		}
	}
	if len(wantCols) == 0 {
		for _, s := range allYieldSeries {
			wantCols[s.key] = true
		}
	}

	// --- check media pipeline ---
	if t.mediaStore == nil {
		return ErrorResult("media store not configured")
	}
	channel := ToolChannel(ctx)
	chatID := ToolChatID(ctx)
	if channel == "" || chatID == "" {
		return ErrorResult("no target channel/chat available — call this tool from an active chat session")
	}

	// --- load plan ---
	plan, err := t.store.GetPlan(ctx, planID)
	if err != nil {
		return ErrorResult(fmt.Sprintf("plan not found: %v", err))
	}

	// --- fetch series ---
	pts, err := t.store.ListSnapshotSeries(ctx, planID, since, 1000)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to load yield history: %v", err))
	}
	if len(pts) == 0 {
		return ErrorResult(fmt.Sprintf("no yield history for plan %d (%s) in the last %s", planID, plan.Name, periodLabel))
	}

	// --- build chart ---
	xVals := make([]time.Time, len(pts))
	for i, p := range pts {
		xVals[i] = p.CheckedAt
	}

	// Decide which requested series go primary vs secondary.
	// If funding_rate is excluded, promote the first APY series to primary so
	// go-chart always has ≥1 primary series.
	hasPrimary := wantCols["funding_rate"]

	var primarySeries []chart.Series
	var secondarySeries []chart.Series

	for _, s := range allYieldSeries {
		if !wantCols[s.key] {
			continue
		}
		yVals := make([]float64, len(pts))
		for i, p := range pts {
			switch s.key {
			case "funding_rate":
				yVals[i] = p.CurrentFundingRate
			case "funding_apy":
				yVals[i] = p.FundingAPYPct
			case "earn_apy":
				yVals[i] = p.EarnAPYPct
			case "combined_apy":
				yVals[i] = p.CombinedAPYPct
			}
		}

		ts := chart.TimeSeries{
			Name: s.name,
			Style: chart.Style{
				StrokeColor: drawing.ColorFromHex(s.hexColor),
				StrokeWidth: 1.5,
				FillColor:   drawing.ColorFromHex(s.hexColor).WithAlpha(15),
			},
			XValues: xVals,
			YValues: yVals,
		}

		isPrimary := !s.secondary || (!hasPrimary && len(primarySeries) == 0)
		if isPrimary {
			primarySeries = append(primarySeries, ts)
		} else {
			secondary := chart.TimeSeries{
				Name:    ts.Name,
				Style:   ts.Style,
				XValues: xVals,
				YValues: yVals,
			}
			secondary.YAxis = chart.YAxisSecondary
			secondarySeries = append(secondarySeries, secondary)
		}
	}

	allSeries := append(primarySeries, secondarySeries...)

	g := chart.Chart{
		Title: fmt.Sprintf("%s — Yield (%s)", plan.Name, periodLabel),
		TitleStyle: chart.Style{
			FontSize: 10,
			Padding:  chart.Box{Top: 6, Left: 6},
		},
		Width:  900,
		Height: 420,
		Background: chart.Style{
			Padding: chart.Box{Top: 40, Left: 10, Right: 10, Bottom: 10},
		},
		XAxis: chart.XAxis{
			Style:          chart.Style{FontSize: 8},
			ValueFormatter: chart.TimeValueFormatterWithFormat("01/02 15:04"),
		},
		YAxis: chart.YAxis{
			Name:      "Funding Rate",
			NameStyle: chart.Style{FontSize: 8},
			Style:     chart.Style{FontSize: 8},
			ValueFormatter: func(v interface{}) string {
				if f, ok := v.(float64); ok {
					return fmt.Sprintf("%.5f", f)
				}
				return fmt.Sprintf("%v", v)
			},
		},
		YAxisSecondary: chart.YAxis{
			Name:      "APY %",
			NameStyle: chart.Style{FontSize: 8},
			Style:     chart.Style{FontSize: 8},
			ValueFormatter: func(v interface{}) string {
				if f, ok := v.(float64); ok {
					return fmt.Sprintf("%.2f%%", f)
				}
				return fmt.Sprintf("%v", v)
			},
		},
		Series: allSeries,
	}
	g.Elements = []chart.Renderable{chart.Legend(&g)}

	// --- render to temp file ---
	tmp, err := os.CreateTemp("", "dn_yield_*.png")
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to create temp file: %v", err))
	}
	defer tmp.Close()

	if err := g.Render(chart.PNG, tmp); err != nil {
		os.Remove(tmp.Name())
		return ErrorResult(fmt.Sprintf("failed to render chart: %v", err))
	}
	_ = tmp.Close()

	// --- register in media store ---
	filename := fmt.Sprintf("dn_yield_%d_%s.png", planID, periodLabel)
	scope := fmt.Sprintf("tool:dn_yield_chart:%s:%s", channel, chatID)
	ref, err := t.mediaStore.Store(tmp.Name(), media.MediaMeta{
		Filename:    filename,
		ContentType: "image/png",
		Source:      "tool:render_delta_neutral_yield_chart",
	}, scope)
	if err != nil {
		os.Remove(tmp.Name())
		return ErrorResult(fmt.Sprintf("failed to register media: %v", err))
	}

	// Build caption: include first→latest combined-APY delta when available.
	caption := fmt.Sprintf("%s — Yield History (%s)", plan.Name, periodLabel)
	if len(pts) >= 2 && wantCols["combined_apy"] {
		first := pts[0].CombinedAPYPct
		last := pts[len(pts)-1].CombinedAPYPct
		delta := last - first
		sign := "+"
		if delta < 0 {
			sign = ""
		}
		caption += fmt.Sprintf("\nCombined APY: %.2f%% → %.2f%% (%s%.2f%%)", first, last, sign, delta)
	}

	return MediaResult(caption, []string{ref}).WithResponseHandled()
}
