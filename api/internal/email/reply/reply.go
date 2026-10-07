// Package reply finds the new text of an e-mail reply: it drops the quoted history that mail
// clients append or prepend and the signature below the delimiter or a "sent from" line.
package reply

import (
	"regexp"
	"strings"
)

var (
	attributionEnd     = regexp.MustCompile(`(?i)(wrote|writes|schrieb|a écrit|escribió|ha scritto|schreef|napisał|skrev|escreveu|yazdı|пишет|написал\(а\)|написал)\s*:\s*$`)
	attributionStart   = regexp.MustCompile(`(?i)^\s*(on|am|le|el|il|op|den|em|w dniu|в)\s`)
	turkishAttribution = regexp.MustCompile(`(?i)(tarihinde|şunu yazdı|yazdı)\s*:?\s*$`)
	separatorLine      = regexp.MustCompile(`(?i)^\s*(-{2,}\s*(original message|ursprüngliche nachricht|message d'origine|mensaje original|özgün ileti|orijinal mesaj|orijinal ileti)\s*-{2,}|_{20,}|-{20,})\s*$`)
	fromHeader         = regexp.MustCompile(`(?i)^\s*\*?(from|kimden|gönderen|von|de|da|van|od|från)\s*\*?\s*:\s*\S`)
	dateHeader         = regexp.MustCompile(`(?i)^\s*\*?(sent|date|gönderildi|gönderilme|tarih|gesendet|envoyé|enviado|inviato|verzonden|wysłano|skickat)\s*\*?\s*:`)
	subjectHeader      = regexp.MustCompile(`(?i)^\s*\*?(subject|konu|betreff|objet|asunto|oggetto|onderwerp|temat|ämne)\s*\*?\s*:`)
	toHeader           = regexp.MustCompile(`(?i)^\s*\*?(to|kime|an|à|para|a|aan|do|till)\s*\*?\s*:`)
	mobileSignature    = regexp.MustCompile(`(?i)^\s*(sent from my \S.*|sent from (outlook|mail|yahoo mail|gmail).*|get outlook for \S.*|sent via \S.*|sent from samsung .*|von meinem \S.* gesendet|envoyé de mon \S.*|enviado desde mi \S.*|iphone'umdan gönderildi|ipad'imden gönderildi|ipad'imdan gönderildi|samsung (galaxy|cep telefonumdan).*gönderildi|android için outlook'u edinin|ios için outlook'u edinin|outlook'u ios için edinin.*|mobil cihazımdan gönderildi)\s*$`)
	forwardMarker      = regexp.MustCompile(`(?i)^\s*(-{2,}\s*(forwarded message|iletilen ileti|weitergeleitete nachricht|message transféré)\s*-{2,}|begin forwarded message:|iletilen ileti başlangıcı:)\s*$`)
	signatureDelimiter = regexp.MustCompile(`^(-- ?|--|—|__)\s*$`)
)

// MaxScan is how much of a message Strip looks at; the new text of a reply comes first, and
// scanning megabytes of crafted header lines would take seconds.
const MaxScan = 64 << 10

// Strip returns the visible part of a plain-text reply and whether anything was hidden. When the
// whole message looks quoted, the full text is returned so nothing the sender wrote is lost.
// Only the first MaxScan bytes are scanned: when nothing is hidden there, the full text is kept.
func Strip(text string) (string, bool) {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	full := strings.TrimSpace(text)
	head := text
	if len(head) > MaxScan {
		head = head[:MaxScan]
		if i := strings.LastIndexByte(head, '\n'); i > 0 {
			head = head[:i]
		}
	}
	kept := removeQuotes(strings.Split(head, "\n"))
	kept = collapseBlank(removeSignature(kept))
	visible := strings.TrimSpace(strings.Join(kept, "\n"))
	if visible == "" || (len(head) < len(text) && visible == strings.TrimSpace(head)) {
		return full, false
	}
	return visible, visible != full
}

func isQuoted(line string) bool {
	return strings.HasPrefix(strings.TrimLeft(line, " \t"), ">")
}

func blank(line string) bool { return strings.TrimSpace(line) == "" }

// attribution reports whether the quote introduction ("On … wrote:") ends on line i, and on
// which line it starts: clients wrap a long introduction over two or three lines.
func attribution(lines []string, i int) (int, bool) {
	l := strings.TrimSpace(lines[i])
	if l == "" || isQuoted(lines[i]) {
		return 0, false
	}
	if !attributionEnd.MatchString(l) && !turkishAttribution.MatchString(l) {
		return 0, false
	}
	for start := i; start >= 0 && start >= i-2; start-- {
		if start < i && (blank(lines[start]) || isQuoted(lines[start])) {
			break
		}
		joined := strings.TrimSpace(strings.Join(trimAll(lines[start:i+1]), " "))
		if attributionStart.MatchString(joined) && attributionEnd.MatchString(joined) {
			return start, true
		}
		if turkishAttribution.MatchString(joined) && (strings.Contains(joined, "@") || hasDigit(joined)) {
			return start, true
		}
	}
	if strings.Contains(l, "@") && attributionEnd.MatchString(l) {
		return i, true
	}
	return 0, false
}

func hasDigit(s string) bool { return strings.ContainsAny(s, "0123456789") }

func trimAll(in []string) []string {
	out := make([]string, len(in))
	for i, l := range in {
		out[i] = strings.TrimSpace(l)
	}
	return out
}

func headerBlock(lines []string, i int) bool {
	if !fromHeader.MatchString(lines[i]) {
		return false
	}
	var date, subject, to bool
	for j := i + 1; j < len(lines) && j <= i+6; j++ {
		l := lines[j]
		switch {
		case dateHeader.MatchString(l):
			date = true
		case subjectHeader.MatchString(l):
			subject = true
		case toHeader.MatchString(l):
			to = true
		}
	}
	return (date && subject) || (date && to) || (subject && to)
}

func nextContent(lines []string, i int) int {
	for i < len(lines) && blank(lines[i]) {
		i++
	}
	return i
}

func removeQuotes(lines []string) []string {
	var out []string
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if forwardMarker.MatchString(line) {
			return append(out, lines[i:]...)
		}
		if separatorLine.MatchString(line) {
			if j := nextContent(lines, i+1); j < len(lines) && (headerBlock(lines, j) || strings.Contains(strings.ToLower(line), "message")) {
				return out
			}
		}
		if headerBlock(lines, i) {
			return dropTrailingSeparator(out)
		}
		if start, ok := attribution(lines, i); ok {
			out = out[:len(out)-(i-start)]
			j := nextContent(lines, i+1)
			if j < len(lines) && isQuoted(lines[j]) {
				for j < len(lines) && (isQuoted(lines[j]) || blank(lines[j])) {
					j++
				}
				i = j - 1
				continue
			}
			return out
		}
		if isQuoted(line) {
			continue
		}
		out = append(out, line)
	}
	return out
}

func dropTrailingSeparator(out []string) []string {
	for len(out) > 0 && (blank(out[len(out)-1]) || separatorLine.MatchString(out[len(out)-1])) {
		out = out[:len(out)-1]
	}
	return out
}

func removeSignature(lines []string) []string {
	for len(lines) > 0 && blank(lines[len(lines)-1]) {
		lines = lines[:len(lines)-1]
	}
	for i, l := range lines {
		if signatureDelimiter.MatchString(l) && i > 0 && len(lines)-i <= 12 {
			return lines[:i]
		}
	}
	tail := 0
	for i := len(lines) - 1; i >= 0 && tail < 3; i-- {
		if blank(lines[i]) {
			continue
		}
		tail++
		if mobileSignature.MatchString(lines[i]) {
			return lines[:i]
		}
	}
	return lines
}

func collapseBlank(lines []string) []string {
	out := lines[:0:0]
	for i, l := range lines {
		if blank(l) && i > 0 && blank(lines[i-1]) {
			continue
		}
		out = append(out, l)
	}
	return out
}
