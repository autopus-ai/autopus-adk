package secretscan

import "regexp"

// localDetectors are shapes neither upstream redactor covers. They run with
// the upstream table in detect, but replaySpans leaves them out: no upstream
// redactor rewrites text with them, so there is nothing to replay. They all
// select spans with plain placeholders (rewriteSpan).
var localDetectors = []detector{
	// A quoted assignment value runs to its closing quote, spaces included;
	// the upstream assignment value stops at the first space.
	newDetector(`(?i)\b[A-Z0-9_.-]*(?:TOKEN|SECRET|PASSWORD|PASSWD|PWD|API[_-]?KEY|PRIVATE[_-]?KEY|ACCESS[_-]?KEY|CREDENTIAL|COOKIE|SESSION|AUTH)[A-Z0-9_.-]*\s*[:=]\s*("[^"\r\n]*"|'[^'\r\n]*')`, kindSecret, quotedInner(1), rewriteSpan),
	// An Authorization header keeps its scheme word; the credentials after
	// it are redacted whatever the scheme (Basic, Digest, Token, ...).
	newDetector(`(?i)\bauthorization\s*[:=]\s*["']?[A-Za-z]+[ \t]+([A-Za-z0-9._~+/=-]{4,})`, kindSecret, group(1), rewriteSpan),
	// A Cookie or Set-Cookie header value is redacted to the end of the
	// line, so every cookie pair is covered, not only the first.
	newDetector(`(?i)\b(?:set-)?cookie\s*:[ \t]*([^\r\n"']+)`, kindSecret, group(1), rewriteSpan),
	// curl user credentials given as -u user:pass or --user user:pass.
	newDetector(`\bcurl\b[^\r\n|;&]*?\s(?:-u|--user)(?:[ \t]+|=)["']?([^\s"':]+:[^\s"']+)`, kindSecret, group(1), rewriteSpan),
	// An OpenPGP private key block, with its armor through the footer.
	newDetector(`-----BEGIN PGP PRIVATE KEY BLOCK-----`, kindSecret, wholeMatch, rewriteSpan).preparedBy(blocksUntil(pgpFooter)),
	// Fixed-prefix vendor tokens.
	newDetector(`\bxox[abprs]-[A-Za-z0-9-]{10,}`, kindSecret, wholeMatch, rewriteSpan).ofAlphabet(),
	newDetector(`\b(?:sk|rk)_(?:live|test)_[A-Za-z0-9]{16,}`, kindSecret, wholeMatch, rewriteSpan).ofAlphabet(),
	newDetector(`\bAIza[0-9A-Za-z_-]{30,}`, kindSecret, wholeMatch, rewriteSpan).ofAlphabet(),
	newDetector(`\bglpat-[A-Za-z0-9_-]{20,}`, kindSecret, wholeMatch, rewriteSpan).ofAlphabet(),
	newDetector(`\bnpm_[A-Za-z0-9]{30,}`, kindSecret, wholeMatch, rewriteSpan).ofAlphabet(),
	// A bare JSON Web Token: base64url header and payload objects ("eyJ")
	// and a signature.
	newDetector(`\beyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`, kindSecret, wholeMatch, rewriteSpan).ofAlphabet(),
}

var pgpFooter = regexp.MustCompile(`-----END PGP PRIVATE KEY BLOCK-----`)

// scanDetectors is every detector detect runs: the upstream table, then the
// local shapes.
var scanDetectors = append(append([]detector(nil), detectors...), localDetectors...)

// quotedInner selects submatch n without the single or double quotes around
// it, so the placeholder sits inside the original quotes.
func quotedInner(n int) selector {
	return func(s string, m []int) (int, int) {
		start, end := m[2*n], m[2*n+1]
		if start >= 0 && end-start >= 2 {
			return start + 1, end - 1
		}
		return start, end
	}
}
