package crawler

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html/charset"
	"golang.org/x/text/encoding"
)

var (
	ErrSSRFBlocked      = errors.New("SSRF protection: IP is private, loopback, or reserved")
	ErrTooManyRedirects = errors.New("too many redirects")
)

// DefaultUserAgent used for SEO requests
const DefaultUserAgent = "AntigravitySEO/2.0 (+https://github.com/antigravity-seo; bot)"

// DefaultAcceptLanguage is a locale-neutral default; override per client for
// localized sites whose servers content-negotiate on this header
const DefaultAcceptLanguage = "en-US,en;q=0.9"

// RedirectHop records details of each hop in a redirect chain
type RedirectHop struct {
	URL        string              `json:"url"`
	StatusCode int                 `json:"status_code"`
	Headers    map[string][]string `json:"headers,omitempty"`
	Timings    FetchTimings        `json:"timings,omitempty"`
}

// FetchTimings records network milestone durations
type FetchTimings struct {
	DNSLookupMS    int64 `json:"dns_lookup_ms"`
	TCPConnectMS   int64 `json:"tcp_connect_ms"`
	TLSHandshakeMS int64 `json:"tls_handshake_ms"`
	TTFBMS         int64 `json:"ttfb_ms"`
	TotalMS        int64 `json:"total_ms"`
}

// FetchResult captures the full transport and body context
type FetchResult struct {
	RequestedURL   string              `json:"requested_url"`
	FinalURL       string              `json:"final_url"`
	StatusCode     int                 `json:"status_code"`
	StatusText     string              `json:"status_text"`
	Headers        map[string][]string `json:"headers"`
	Redirects      []RedirectHop       `json:"redirect_chain,omitempty"`
	Timings        FetchTimings        `json:"timings"`
	TotalElapsedMS int64               `json:"total_elapsed_ms,omitempty"`
	Body           []byte              `json:"-"`
	BodySize       int                 `json:"body_size_bytes"`
	ContentType    string              `json:"content_type"`
	Charset        string              `json:"charset,omitempty"`
	Truncated      bool                `json:"truncated,omitempty"`
}

// Resolver looks up the IP addresses for host. It is injectable so tests can
// simulate DNS results (including rebinding and private-IP scenarios)
// without depending on real network resolution.
type Resolver func(ctx context.Context, host string) ([]net.IP, error)

// ClientOptions configures safe network behavior
type ClientOptions struct {
	UserAgent       string
	AcceptLanguage  string
	Timeout         time.Duration
	MaxRedirects    int
	MaxBodyBytes    int64
	AllowPrivateIPs bool // Set true only in testing environments

	// Resolver overrides DNS resolution used for SSRF validation and
	// dialing. Defaults to net.DefaultResolver.LookupIPAddr.
	Resolver Resolver
	// ProxyFunc overrides proxy selection (defaults to http.ProxyFromEnvironment).
	// Exposed for tests that need to simulate a request being proxied.
	ProxyFunc func(*http.Request) (*url.URL, error)
}

// SafeClient manages safe requests with SSRF guards and redirect inspection
type SafeClient struct {
	httpClient *http.Client
	options    ClientOptions
}

// NewSafeClient initializes a configured SafeClient
func NewSafeClient(opts ClientOptions) *SafeClient {
	if opts.UserAgent == "" {
		opts.UserAgent = DefaultUserAgent
	}
	if opts.AcceptLanguage == "" {
		opts.AcceptLanguage = DefaultAcceptLanguage
	}
	if opts.Timeout == 0 {
		opts.Timeout = 15 * time.Second
	}
	if opts.MaxRedirects == 0 {
		opts.MaxRedirects = 10
	}
	if opts.MaxBodyBytes == 0 {
		opts.MaxBodyBytes = 15 * 1024 * 1024 // 15MB cap
	}
	if opts.Resolver == nil {
		opts.Resolver = defaultResolver
	}
	if opts.ProxyFunc == nil {
		opts.ProxyFunc = http.ProxyFromEnvironment
	}

	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		Proxy: opts.ProxyFunc,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}

			if opts.AllowPrivateIPs {
				return dialer.DialContext(ctx, network, addr)
			}

			// When this request goes through an operator-configured proxy, addr
			// here is the PROXY's address, not the crawl target: it is trusted
			// as-is (a localhost proxy must not be blocked), and the real target
			// host was already resolved and validated in Fetch before Do() was
			// called. See proxyTrustedKey.
			if trusted, _ := ctx.Value(proxyTrustedKey).(bool); trusted {
				return dialer.DialContext(ctx, network, addr)
			}

			ips, err := opts.Resolver(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("DNS lookup failed: %w", err)
			}

			// Dial the already-validated IP literal directly rather than the
			// hostname again: re-resolving here would reopen a DNS-rebinding
			// window between validation and connect (TOCTOU).
			var lastErr error
			for _, ip := range ips {
				if isBlockedIP(ip) {
					lastErr = fmt.Errorf("%w: %s (%s)", ErrSSRFBlocked, host, ip)
					continue
				}
				conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if dialErr == nil {
					return conn, nil
				}
				lastErr = dialErr
			}
			if lastErr == nil {
				lastErr = fmt.Errorf("%w: no addresses resolved for %s", ErrSSRFBlocked, host)
			}
			return nil, lastErr
		},
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &SafeClient{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   opts.Timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		options: opts,
	}
}

// ctxKey namespaces context values set by this package.
type ctxKey int

// proxyTrustedKey marks a request context as routed through an
// operator-configured proxy whose own address should not be SSRF-validated
// (the proxy is trusted; the real target is validated separately in Fetch).
const proxyTrustedKey ctxKey = 0

// defaultResolver is the production Resolver, backed by net.DefaultResolver.
func defaultResolver(ctx context.Context, host string) ([]net.IP, error) {
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, len(addrs))
	for i, a := range addrs {
		ips[i] = a.IP
	}
	return ips, nil
}

// validateTargetHost resolves host and rejects it if any address is blocked.
// Used to validate the real crawl target when a proxy is in play, since in
// that case DialContext only ever sees the proxy's own address.
func (c *SafeClient) validateTargetHost(ctx context.Context, host string) error {
	ips, err := c.options.Resolver(ctx, host)
	if err != nil {
		return fmt.Errorf("DNS lookup failed: %w", err)
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return fmt.Errorf("%w: %s (%s)", ErrSSRFBlocked, host, ip)
		}
	}
	return nil
}

// isBlockedIP checks if an IP belongs to private, loopback, multicast, or cloud metadata ranges
func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsMulticast() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	// Cloud metadata IP: 169.254.169.254
	metadataIP := net.ParseIP("169.254.169.254")
	if ip.Equal(metadataIP) {
		return true
	}

	// Carrier Grade NAT (100.64.0.0/10)
	_, cgnat, _ := net.ParseCIDR("100.64.0.0/10")
	if cgnat != nil && cgnat.Contains(ip) {
		return true
	}

	return false
}

// Fetch retrieves a URL, following redirects and recording the full audit trail
func (c *SafeClient) Fetch(ctx context.Context, targetURL string) (*FetchResult, error) {
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("unsupported protocol: %s", parsed.Scheme)
	}

	var (
		redirects  []RedirectHop
		currentURL = targetURL
		chainStart = time.Now()
	)

	// Step-by-step redirect follower to accurately capture every hop's headers and status
	for hopCount := 0; hopCount <= c.options.MaxRedirects; hopCount++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, currentURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", c.options.UserAgent)
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", c.options.AcceptLanguage)

		if !c.options.AllowPrivateIPs {
			// If this request would be routed through a proxy, DialContext will
			// only ever see the proxy's own address, not this hop's target host.
			// Validate the target here so a proxy can't be used to reach a
			// private/blocked address.
			if proxyURL, perr := c.options.ProxyFunc(req); perr == nil && proxyURL != nil {
				if verr := c.validateTargetHost(ctx, parsed.Hostname()); verr != nil {
					return nil, verr
				}
				req = req.WithContext(context.WithValue(req.Context(), proxyTrustedKey, true))
			}
		}

		var (
			dnsStart, tcpStart, tlsStart, ttfbStart time.Time
			timings                                 FetchTimings
		)

		trace := &httptrace.ClientTrace{
			DNSStart: func(_ httptrace.DNSStartInfo) { dnsStart = time.Now() },
			DNSDone: func(_ httptrace.DNSDoneInfo) {
				if !dnsStart.IsZero() {
					timings.DNSLookupMS = time.Since(dnsStart).Milliseconds()
				}
			},
			ConnectStart: func(_, _ string) { tcpStart = time.Now() },
			ConnectDone: func(_, _ string, _ error) {
				if !tcpStart.IsZero() {
					timings.TCPConnectMS = time.Since(tcpStart).Milliseconds()
				}
			},
			TLSHandshakeStart: func() { tlsStart = time.Now() },
			TLSHandshakeDone: func(_ tls.ConnectionState, _ error) {
				if !tlsStart.IsZero() {
					timings.TLSHandshakeMS = time.Since(tlsStart).Milliseconds()
				}
			},
			GotFirstResponseByte: func() {
				if !ttfbStart.IsZero() {
					timings.TTFBMS = time.Since(ttfbStart).Milliseconds()
				}
			},
		}

		overallStart := time.Now()
		ttfbStart = overallStart
		req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		timings.TotalMS = time.Since(overallStart).Milliseconds()

		// If redirect (301, 302, 303, 307, 308)
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			loc := resp.Header.Get("Location")
			_ = resp.Body.Close()

			if loc == "" {
				return nil, fmt.Errorf("redirect status %d without Location header", resp.StatusCode)
			}

			// Resolve relative redirect
			nextURL, err := parsed.Parse(loc)
			if err != nil {
				return nil, fmt.Errorf("invalid redirect Location %q: %w", loc, err)
			}

			redirects = append(redirects, RedirectHop{
				URL:        currentURL,
				StatusCode: resp.StatusCode,
				Headers:    resp.Header,
				Timings:    timings,
			})

			currentURL = nextURL.String()
			parsed = nextURL
			continue
		}

		// Terminal response
		defer resp.Body.Close()
		// Read one byte past the cap so we can tell a truncated body apart
		// from one that happens to end exactly at the limit.
		limitedReader := io.LimitReader(resp.Body, c.options.MaxBodyBytes+1)
		bodyBytes, err := io.ReadAll(limitedReader)
		if err != nil {
			return nil, fmt.Errorf("failed reading body: %w", err)
		}
		truncated := false
		if int64(len(bodyBytes)) > c.options.MaxBodyBytes {
			truncated = true
			bodyBytes = bodyBytes[:c.options.MaxBodyBytes]
		}

		if resp.Uncompressed && len(resp.Header["Content-Encoding"]) == 0 {
			resp.Header["Content-Encoding"] = []string{"gzip"}
		}

		rawContentType := resp.Header.Get("Content-Type")
		bodyBytes, charsetName := decodeToUTF8(bodyBytes, rawContentType, parsed.Path)

		return &FetchResult{
			RequestedURL:   targetURL,
			FinalURL:       currentURL,
			StatusCode:     resp.StatusCode,
			StatusText:     http.StatusText(resp.StatusCode),
			Headers:        resp.Header,
			Redirects:      redirects,
			Timings:        timings,
			TotalElapsedMS: time.Since(chainStart).Milliseconds(),
			Body:           bodyBytes,
			BodySize:       len(bodyBytes),
			ContentType:    strings.TrimSpace(strings.Split(rawContentType, ";")[0]),
			Charset:        charsetName,
			Truncated:      truncated,
		}, nil
	}

	return nil, ErrTooManyRedirects
}

// xmlEncodingDeclRe extracts the encoding attribute from an XML prolog, e.g.
// <?xml version="1.0" encoding="Shift_JIS"?>.
var xmlEncodingDeclRe = regexp.MustCompile(`(?i)<\?xml[^>?]*\bencoding\s*=\s*["']([^"']+)["']`)

// decodeToUTF8 transcodes body to UTF-8 when it is HTML declaring (or
// sniffed as) a non-UTF-8 charset, or an XML document whose prolog declares
// one. It returns the (possibly unchanged) body and the detected charset
// label, empty if none was determined. Anything else is left untouched.
func decodeToUTF8(body []byte, rawContentType, path string) ([]byte, string) {
	mediaType, _, _ := mime.ParseMediaType(rawContentType)
	isHTML := mediaType == "text/html" || mediaType == "application/xhtml+xml"
	if !isHTML && strings.HasPrefix(http.DetectContentType(body), "text/html") {
		isHTML = true
	}

	if isHTML {
		enc, name, _ := charset.DetermineEncoding(body, rawContentType)
		if enc == encoding.Nop {
			return body, name
		}
		if decoded, err := enc.NewDecoder().Bytes(body); err == nil {
			return decoded, name
		}
		return body, name
	}

	isXML := strings.Contains(mediaType, "xml") || strings.HasSuffix(strings.ToLower(path), ".xml")
	if isXML {
		head := body
		if len(head) > 512 {
			head = head[:512]
		}
		m := xmlEncodingDeclRe.FindSubmatch(head)
		if m == nil {
			return body, ""
		}
		label := string(m[1])
		switch strings.ToLower(label) {
		case "utf-8", "utf8", "us-ascii", "ascii":
			return body, "utf-8"
		}
		enc, name := charset.Lookup(label)
		if enc == nil {
			return body, ""
		}
		if decoded, err := enc.NewDecoder().Bytes(body); err == nil {
			return decoded, name
		}
	}

	return body, ""
}
