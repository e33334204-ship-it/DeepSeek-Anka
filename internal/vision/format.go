package vision

import (
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var attachedImageRe = regexp.MustCompile(`\[attached_image:\s*([^\]]+)\]`)

// UniqueImagePathsFromText extracts image paths from [attached_image: ...] markers
// and @.deepseek-anka/attachments/... references (Reasonix composer format).
func UniqueImagePathsFromText(text string) []string {
	seen := map[string]bool{}
	var paths []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		paths = append(paths, p)
	}
	for _, m := range attachedImageRe.FindAllStringSubmatch(text, -1) {
		add(m[1])
	}
	// Reasonix attachment refs: @.deepseek-anka/attachments/foo.png
	refRe := regexp.MustCompile(`@(\.deepseek-anka/attachments/[^\s]+)`)
	for _, m := range refRe.FindAllStringSubmatch(text, -1) {
		add(m[1])
	}
	return paths
}

func normalizeUserRequest(text string) string {
	s := attachedImageRe.ReplaceAllString(text, "")
	s = regexp.MustCompile(`@\.deepseek-anka/attachments/[^\s]+`).ReplaceAllString(s, "")
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(s)
}

// FormatVisionContext builds the injectable vision-context block.
func FormatVisionContext(notes []PreparedNote) string {
	if len(notes) == 0 {
		return ""
	}
	var lines []string
	for i, n := range notes {
		label := ""
		if n.Label != "" && n.Label != n.Key {
			label = " (" + n.Label + ")"
		}
		lines = append(lines, "image_"+strconv.Itoa(i+1)+label+": "+n.Note)
	}
	return ContextStart + "\n" + strings.Join(lines, "\n\n") + "\n" + ContextEnd + "\n\n"
}

// WrapNote wraps a single note in vision-context tags.
func WrapNote(note string) string {
	note = strings.TrimSpace(note)
	if note == "" {
		return ""
	}
	return ContextStart + "\n" + note + "\n" + ContextEnd
}

func truncateText(text string, max int) string {
	text = strings.TrimSpace(text)
	if len(text) <= max {
		return text
	}
	return text[:max-20] + "\n[truncated]"
}

func safeSection(value any, fallback string) string {
	switch v := value.(type) {
	case []any:
		var parts []string
		for _, item := range v {
			if s := strings.TrimSpace(toString(item)); s != "" {
				parts = append(parts, s)
			}
		}
		if joined := strings.Join(parts, "; "); joined != "" {
			return joined
		}
	case []string:
		var parts []string
		for _, s := range v {
			if s = strings.TrimSpace(s); s != "" {
				parts = append(parts, s)
			}
		}
		if joined := strings.Join(parts, "; "); joined != "" {
			return joined
		}
	default:
		if s := strings.TrimSpace(toString(v)); s != "" {
			return s
		}
	}
	if fallback == "" {
		return "none"
	}
	return fallback
}

func toString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	default:
		return fmtAny(v)
	}
}

func fmtAny(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

func extractJSONObject(text string) map[string]any {
	raw := strings.TrimSpace(text)
	if raw == "" {
		return nil
	}
	fenced := regexp.MustCompile("(?is)```(?:json)?\\s*([\\s\\S]*?)```").FindStringSubmatch(raw)
	primary := raw
	if len(fenced) > 1 {
		primary = strings.TrimSpace(fenced[1])
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(primary), &parsed); err == nil {
		return parsed
	}
	start := strings.Index(primary, "{")
	end := strings.LastIndex(primary, "}")
	if start < 0 || end <= start {
		return nil
	}
	if err := json.Unmarshal([]byte(primary[start:end+1]), &parsed); err != nil {
		return nil
	}
	return parsed
}

func formatStructuredVisionNote(analysis map[string]any, caps *VisionCapabilities) string {
	primitives := normalizeVisualPrimitives(rawVisualPrimitiveItems(analysis), caps)
	sections := []string{
		"image_overview: " + safeSection(analysis["image_overview"], "none"),
		"visible_text: " + safeSection(analysis["visible_text"], "none"),
		"objects_and_layout: " + safeSection(analysis["objects_and_layout"], "none"),
		"charts_or_data: " + safeSection(analysis["charts_or_data"], "none"),
		"user_request: " + safeSection(analysis["user_request"], "none"),
		"user_request_answer: " + safeSection(analysis["user_request_answer"], "none"),
		"evidence: " + safeSection(analysis["evidence"], "none"),
		"uncertainty: " + safeSection(analysis["uncertainty"], "none"),
	}
	grounding := "unavailable"
	if caps != nil && caps.GroundingMode != "" {
		grounding = caps.GroundingMode
	}
	primitiveBlock := formatVisualPrimitives(primitives, grounding)
	return truncateText(strings.Join(sections, "\n")+"\n\n"+primitiveBlock, maxNoteChars)
}

func formatInvalidStructuredNote(rawResponse string) string {
	primitiveBlock := formatVisualPrimitives(nil, "unavailable")
	return truncateText(strings.Join([]string{
		"image_overview: structured vision analysis unavailable.",
		"visible_text: none.",
		"objects_and_layout: none.",
		"charts_or_data: none.",
		"user_request: see original message.",
		"user_request_answer: The auxiliary vision model returned invalid structured JSON, so coordinate evidence was not used.",
		"evidence: raw response excerpt: " + truncateText(rawResponse, 700),
		"uncertainty: visual primitives unavailable because the response could not be parsed.",
		"",
		primitiveBlock,
	}, "\n"), maxNoteChars)
}

func rawVisualPrimitiveItems(analysis map[string]any) []any {
	for _, key := range []string{"visual_primitives", "visual_anchors", "anchors"} {
		if items, ok := analysis[key].([]any); ok {
			return items
		}
	}
	return nil
}

type visualPrimitive struct {
	ID          string
	Type        string
	Ref         string
	Box         []int
	Point       []int
	Confidence  *float64
	Grounding   string
}

func formatVisualPrimitives(primitives []visualPrimitive, groundingMode string) string {
	if len(primitives) == 0 {
		return strings.Join([]string{
			VisualPrimitivesStart + ` coord="norm-1000" box_order="xyxy" grounding="unavailable">`,
			"- unavailable | reason: no valid coordinates",
			VisualPrimitivesEnd,
		}, "\n")
	}
	var lines []string
	for _, p := range primitives {
		coord := ""
		if p.Type == "box" && len(p.Box) == 4 {
			coord = "box: [" + joinInts(p.Box) + "]"
		} else if p.Type == "point" && len(p.Point) == 2 {
			coord = "point: [" + joinInts(p.Point) + "]"
		}
		conf := ""
		if p.Confidence != nil {
			conf = " | confidence: " + strconv.FormatFloat(*p.Confidence, 'f', 2, 64)
		}
		lines = append(lines, "- "+p.ID+" | type: "+p.Type+" | "+coord+" | ref: "+p.Ref+conf+" | grounding: "+p.Grounding)
	}
	return strings.Join(append([]string{
		VisualPrimitivesStart + ` coord="norm-1000" box_order="xyxy" grounding="` + groundingMode + `">`,
	}, append(lines, VisualPrimitivesEnd)...), "\n")
}

func joinInts(v []int) string {
	parts := make([]string, len(v))
	for i, n := range v {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ", ")
}

func normalizeVisualPrimitives(items []any, caps *VisionCapabilities) []visualPrimitive {
	if caps == nil || len(items) == 0 {
		return nil
	}
	var out []visualPrimitive
	for i := 0; i < len(items) && len(out) < maxVisualPrimitives; i++ {
		raw, ok := items[i].(map[string]any)
		if !ok {
			continue
		}
		if p := normalizePrimitive(raw, i, caps); p != nil {
			out = append(out, *p)
		}
	}
	return out
}

func normalizePrimitive(raw map[string]any, index int, caps *VisionCapabilities) *visualPrimitive {
	rawBox := firstArray(raw, "box", "bbox", "bbox_2d", "box_2d")
	rawPoint := firstArray(raw, "point", "point_2d", "center")

	if caps.Boxes {
		if box := normalizeBox(rawBox, caps); box != nil {
			return &visualPrimitive{
				ID: primitiveID(raw, index), Type: "box",
				Ref: primitiveLabel(raw, index), Box: box,
				Confidence: normalizeConfidence(raw["confidence"]),
				Grounding:  caps.GroundingMode,
			}
		}
	}
	if caps.Points {
		if point := normalizePoint(rawPoint); point != nil {
			return &visualPrimitive{
				ID: primitiveID(raw, index), Type: "point",
				Ref: primitiveLabel(raw, index), Point: point,
				Confidence: normalizeConfidence(raw["confidence"]),
				Grounding:  caps.GroundingMode,
			}
		}
	}
	return nil
}

func firstArray(raw map[string]any, keys ...string) []any {
	for _, k := range keys {
		if v, ok := raw[k].([]any); ok {
			return v
		}
	}
	return nil
}

func normalizeBox(rawBox []any, caps *VisionCapabilities) []int {
	if len(rawBox) != 4 {
		return nil
	}
	coords := make([]int, 4)
	for i, v := range rawBox {
		n := clampNorm(v)
		if n == nil {
			return nil
		}
		coords[i] = *n
	}
	var x1, y1, x2, y2 int
	if caps.BoxOrder == "yxyx" {
		y1, x1, y2, x2 = coords[0], coords[1], coords[2], coords[3]
	} else {
		x1, y1, x2, y2 = coords[0], coords[1], coords[2], coords[3]
	}
	left, top := min(x1, x2), min(y1, y2)
	right, bottom := max(x1, x2), max(y1, y2)
	if left == right || top == bottom {
		return nil
	}
	return []int{left, top, right, bottom}
}

func normalizePoint(rawPoint []any) []int {
	if len(rawPoint) != 2 {
		return nil
	}
	out := make([]int, 2)
	for i, v := range rawPoint {
		n := clampNorm(v)
		if n == nil {
			return nil
		}
		out[i] = *n
	}
	return out
}

func clampNorm(value any) *int {
	var n float64
	switch v := value.(type) {
	case float64:
		n = v
	case json.Number:
		f, _ := v.Float64()
		n = f
	case int:
		n = float64(v)
	default:
		return nil
	}
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return nil
	}
	clamped := int(math.Max(0, math.Min(1000, math.Round(n))))
	return &clamped
}

func normalizeConfidence(value any) *float64 {
	if value == nil || value == "" {
		return nil
	}
	var n float64
	switch v := value.(type) {
	case float64:
		n = v
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return nil
		}
		n = f
	default:
		return nil
	}
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return nil
	}
	clamped := math.Max(0, math.Min(1, n))
	return &clamped
}

func primitiveLabel(raw map[string]any, index int) string {
	fallback := "v" + strconv.Itoa(index+1)
	for _, key := range []string{"ref", "label", "text", "name", "id"} {
		if s := strings.TrimSpace(toString(raw[key])); s != "" {
			s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
			if len(s) > maxPrimitiveRefChars {
				s = s[:maxPrimitiveRefChars]
			}
			return s
		}
	}
	return fallback
}

func primitiveID(raw map[string]any, index int) string {
	fallback := "v" + strconv.Itoa(index+1)
	candidate := strings.TrimSpace(toString(raw["id"]))
	if candidate == "" {
		return fallback
	}
	candidate = regexp.MustCompile(`[^a-zA-Z0-9_-]`).ReplaceAllString(candidate, "_")
	if candidate == "" {
		return fallback
	}
	return candidate
}

func primitivePromptShape(caps *VisionCapabilities) []string {
	if caps == nil {
		return []string{
			`  "visual_primitives": [`,
			`    {"id":"v1","type":"box","ref":"short label","box":[0,0,0,0],"confidence":0.0}`,
			`  ]`,
			`For boxes, output the box array as [x1, y1, x2, y2] normalized to 0-1000.`,
		}
	}
	switch caps.OutputFormat {
	case "gemini":
		return []string{
			`  "visual_primitives": [`,
			`    {"id":"v1","type":"box","label":"short label","box_2d":[0,0,0,0],"confidence":0.0}`,
			`  ]`,
			"For Gemini-family models, use box_2d with the native [ymin, xmin, ymax, xmax] order normalized to 0-1000.",
		}
	case "qwen":
		return []string{
			`  "visual_primitives": [`,
			`    {"id":"v1","label":"short label","bbox_2d":[0,0,0,0],"point_2d":[0,0],"confidence":0.0}`,
			`  ]`,
			"For Qwen-family models, use bbox_2d as [x1, y1, x2, y2] and point_2d as [x, y], normalized to 0-1000.",
		}
	case "anchor":
		return []string{
			`  "visual_anchors": [`,
			`    {"id":"v1","label":"short label","role":"button|text|object|region","center":[0,0],"box":[0,0,0,0],"confidence":0.0}`,
			`  ]`,
			"For computer-use style models, prefer visual_anchors with center [x, y] for clickable or salient targets, plus box [x1, y1, x2, y2] when visible.",
		}
	default:
		order := "[x1, y1, x2, y2]"
		if caps.BoxOrder == "yxyx" {
			order = "[ymin, xmin, ymax, xmax]"
		}
		return []string{
			`  "visual_primitives": [`,
			`    {"id":"v1","type":"box","ref":"short label","box":[0,0,0,0],"confidence":0.0}`,
			`  ]`,
			"For boxes, output the box array as " + order + " normalized to 0-1000.",
		}
	}
}
