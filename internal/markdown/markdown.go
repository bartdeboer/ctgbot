package markdown

import (
	"fmt"
	"html"
	"strings"
	"unicode/utf8"
)

type RenderFormat string

const (
	RenderPlain RenderFormat = "plain"
	RenderHTML  RenderFormat = "html"
)

type RenderOptions struct {
	Format    RenderFormat
	ChunkSize int
}

type Chunk struct {
	Text string
}

type Doc struct {
	blocks []block
}

type blockKind int

const (
	blockParagraph blockKind = iota
	blockCode
)

type block struct {
	kind     blockKind
	segments []segment
	code     string
}

type Style uint8

const (
	StyleBold Style = 1 << iota
	StyleItalic
	StyleCode
)

type segment struct {
	text  string
	style Style
}

func New(text string) *Doc {
	return &Doc{blocks: parseBlocks(normalize(text))}
}

func (d *Doc) RenderChunked(opts RenderOptions) ([]Chunk, error) {
	format := opts.Format
	if format == "" {
		format = RenderPlain
	}
	if format != RenderPlain && format != RenderHTML {
		return nil, fmt.Errorf("unsupported render format %q", format)
	}
	limit := opts.ChunkSize
	if limit <= 0 {
		limit = 3500
	}
	if d == nil || len(d.blocks) == 0 {
		return []Chunk{{Text: ""}}, nil
	}

	pieces := make([]string, 0, len(d.blocks))
	for _, blk := range d.blocks {
		var blockPieces []string
		switch blk.kind {
		case blockCode:
			blockPieces = renderCodeBlock(blk.code, format, limit)
		default:
			blockPieces = renderParagraph(blk.segments, format, limit)
		}
		for _, piece := range blockPieces {
			if strings.TrimSpace(piece) == "" {
				continue
			}
			pieces = append(pieces, piece)
		}
	}
	if len(pieces) == 0 {
		return []Chunk{{Text: ""}}, nil
	}

	var chunks []Chunk
	current := ""
	for _, piece := range pieces {
		if current == "" {
			current = piece
			continue
		}
		candidate := current + "\n\n" + piece
		if textLen(candidate) <= limit {
			current = candidate
			continue
		}
		chunks = append(chunks, Chunk{Text: current})
		current = piece
	}
	if current != "" {
		chunks = append(chunks, Chunk{Text: current})
	}
	return chunks, nil
}

func normalize(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.TrimSpace(text)
}

func parseBlocks(text string) []block {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	var blocks []block
	var paragraph []string
	flushParagraph := func() {
		joined := strings.TrimSpace(strings.Join(paragraph, "\n"))
		paragraph = nil
		if joined == "" {
			return
		}
		blocks = append(blocks, block{kind: blockParagraph, segments: parseSegments(joined, 0)})
	}

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			flushParagraph()
			var code []string
			for i++; i < len(lines); i++ {
				if strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
					break
				}
				code = append(code, lines[i])
			}
			blocks = append(blocks, block{kind: blockCode, code: strings.Trim(strings.Join(code, "\n"), "\n")})
			continue
		}
		if trimmed == "" {
			flushParagraph()
			continue
		}
		paragraph = append(paragraph, line)
	}
	flushParagraph()
	return blocks
}

func parseSegments(text string, style Style) []segment {
	if text == "" {
		return nil
	}
	var out []segment
	for len(text) > 0 {
		switch {
		case strings.HasPrefix(text, "**"):
			end := strings.Index(text[2:], "**")
			if end >= 0 {
				inner := text[2 : 2+end]
				out = append(out, parseSegments(inner, style|StyleBold)...)
				text = text[2+end+2:]
				continue
			}
		case strings.HasPrefix(text, "*"):
			end := strings.Index(text[1:], "*")
			if end >= 0 {
				inner := text[1 : 1+end]
				out = append(out, parseSegments(inner, style|StyleItalic)...)
				text = text[1+end+1:]
				continue
			}
		case strings.HasPrefix(text, "`"):
			end := strings.Index(text[1:], "`")
			if end >= 0 {
				inner := text[1 : 1+end]
				out = append(out, segment{text: inner, style: StyleCode})
				text = text[1+end+1:]
				continue
			}
		}

		next := nextMarker(text)
		out = append(out, segment{text: text[:next], style: style})
		text = text[next:]
	}
	return mergeSegments(out)
}

func nextMarker(text string) int {
	indexes := []int{}
	for _, marker := range []string{"**", "*", "`"} {
		if idx := strings.Index(text, marker); idx >= 0 {
			indexes = append(indexes, idx)
		}
	}
	if len(indexes) == 0 {
		return len(text)
	}
	min := len(text)
	for _, idx := range indexes {
		if idx < min {
			min = idx
		}
	}
	if min == 0 {
		return len(text)
	}
	return min
}

func mergeSegments(in []segment) []segment {
	out := make([]segment, 0, len(in))
	for _, seg := range in {
		if seg.text == "" {
			continue
		}
		if n := len(out); n > 0 && out[n-1].style == seg.style {
			out[n-1].text += seg.text
			continue
		}
		out = append(out, seg)
	}
	return out
}

func renderParagraph(segments []segment, format RenderFormat, limit int) []string {
	if len(segments) == 0 {
		return nil
	}
	var pieces []string
	current := make([]segment, 0, len(segments))
	currentLen := 0

	flush := func() {
		if len(current) == 0 {
			return
		}
		pieces = append(pieces, renderSegments(current, format))
		current = nil
		currentLen = 0
	}

	for _, seg := range segments {
		remaining := seg.text
		for remaining != "" {
			available := limit - currentLen
			if available <= 0 {
				flush()
				available = limit
			}
			pieceText, rest := splitTextForLimit(remaining, available-segmentWrapperLen(seg.style, format))
			if pieceText == "" {
				flush()
				pieceText, rest = splitTextForLimit(remaining, limit-segmentWrapperLen(seg.style, format))
			}
			piece := segment{text: pieceText, style: seg.style}
			pieceLen := renderedSegmentLen(piece, format)
			if pieceLen > limit && currentLen == 0 {
				pieces = append(pieces, renderSegments([]segment{piece}, format))
				remaining = rest
				continue
			}
			if currentLen > 0 && currentLen+pieceLen > limit {
				flush()
				continue
			}
			current = append(current, piece)
			currentLen += pieceLen
			remaining = rest
		}
	}
	flush()
	return pieces
}

func renderCodeBlock(code string, format RenderFormat, limit int) []string {
	lines := strings.Split(code, "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}
	var pieces []string
	current := make([]string, 0, len(lines))
	flush := func() {
		if len(current) == 0 {
			return
		}
		pieces = append(pieces, renderCodeLines(current, format))
		current = nil
	}
	for _, line := range lines {
		candidate := append(append([]string(nil), current...), line)
		if textLen(renderCodeLines(candidate, format)) <= limit || len(current) == 0 {
			current = append(current, line)
			if textLen(renderCodeLines(current, format)) <= limit {
				continue
			}
		}
		if len(current) > 1 {
			last := current[len(current)-1]
			current = current[:len(current)-1]
			flush()
			current = []string{last}
		}
		for textLen(renderCodeLines(current, format)) > limit {
			part, rest := splitTextForLimit(current[0], limit-codeWrapperLen(format))
			if part == "" {
				part, rest = forceSplit(current[0], maxInt(1, limit-codeWrapperLen(format)))
			}
			pieces = append(pieces, renderCodeLines([]string{part}, format))
			current[0] = rest
		}
	}
	flush()
	return pieces
}

func renderCodeLines(lines []string, format RenderFormat) string {
	joined := strings.Join(lines, "\n")
	switch format {
	case RenderHTML:
		return "<pre><code>" + html.EscapeString(joined) + "</code></pre>"
	default:
		return joined
	}
}

func renderSegments(segments []segment, format RenderFormat) string {
	var b strings.Builder
	for _, seg := range segments {
		b.WriteString(renderSegment(seg, format))
	}
	return strings.TrimSpace(b.String())
}

func renderSegment(seg segment, format RenderFormat) string {
	if seg.text == "" {
		return ""
	}
	switch format {
	case RenderHTML:
		return wrapHTML(seg.text, seg.style)
	default:
		return seg.text
	}
}

func wrapHTML(text string, style Style) string {
	escaped := html.EscapeString(text)
	if style&StyleCode != 0 {
		return "<code>" + escaped + "</code>"
	}
	if style&StyleItalic != 0 {
		escaped = "<i>" + escaped + "</i>"
	}
	if style&StyleBold != 0 {
		escaped = "<b>" + escaped + "</b>"
	}
	return escaped
}

func renderedSegmentLen(seg segment, format RenderFormat) int {
	return textLen(renderSegment(seg, format))
}

func segmentWrapperLen(style Style, format RenderFormat) int {
	if format != RenderHTML {
		return 0
	}
	switch {
	case style&StyleCode != 0:
		return len("<code></code>")
	default:
		n := 0
		if style&StyleItalic != 0 {
			n += len("<i></i>")
		}
		if style&StyleBold != 0 {
			n += len("<b></b>")
		}
		return n
	}
}

func codeWrapperLen(format RenderFormat) int {
	if format == RenderHTML {
		return len("<pre><code></code></pre>")
	}
	return 0
}

func splitTextForLimit(text string, limit int) (string, string) {
	if text == "" {
		return "", ""
	}
	if limit <= 0 {
		return "", text
	}
	if textLen(text) <= limit {
		return text, ""
	}
	cut := bestCut(text, limit)
	if cut <= 0 {
		return forceSplit(text, limit)
	}
	return strings.TrimSpace(text[:cut]), strings.TrimSpace(text[cut:])
}

func bestCut(text string, limit int) int {
	if textLen(text) <= limit {
		return len(text)
	}
	cut := byteIndexAtRuneLimit(text, limit)
	window := text[:cut]
	for _, needle := range []string{"\n", ". ", " ", ", ", "; ", ": "} {
		if idx := strings.LastIndex(window, needle); idx > 0 {
			if needle == "\n" {
				return idx
			}
			return idx + len(needle) - 1
		}
	}
	return cut
}

func forceSplit(text string, limit int) (string, string) {
	cut := byteIndexAtRuneLimit(text, limit)
	if cut <= 0 || cut >= len(text) {
		return text, ""
	}
	return strings.TrimSpace(text[:cut]), strings.TrimSpace(text[cut:])
}

func byteIndexAtRuneLimit(text string, limit int) int {
	if limit <= 0 {
		return 0
	}
	count := 0
	for idx := range text {
		if count == limit {
			return idx
		}
		count++
	}
	return len(text)
}

func textLen(text string) int {
	return utf8.RuneCountInString(text)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
