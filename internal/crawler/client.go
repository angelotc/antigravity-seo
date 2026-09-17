package crawler

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"time"
)

var (
	ErrSSRFBlocked     = errors.New("SSRF protection: IP is private, loopback, or reserved")
	ErrTooManyRedirects = errors.New("too many redirects")
)

// DefaultUserAgent used for SEO requests
const DefaultUserAgent = "AntigravitySEO/1.0 (+https://github.com/antigravity-seo; bot)"

// RedirectHop records details of each hop in a redirect chain
type RedirectHop struct {
	URL        string              `json:"url"`
	StatusCode int                 `json:"status_code"`
	Headers    map[string][]string `json:"headers,omitempty"`
}

// FetchTimings records network milestone durations
type FetchTimings struct {
	DNSLookupMS  int64 `json:"dns_lookup_ms"`
	TCPConnectMS int64 `json:"tcp_connect_ms"`
	TLSHandshakeMS int64 `json:"tls_handshake_ms"`
	TTFBMS       int64 `json:"ttfb_ms"`
	TotalMS      int64 `json:"total_ms"`
}

// FetchResult captures the full transport and body context
type FetchResult struct {
	RequestedURL string              `json:"requested_url"`
	FinalURL     string              `json:"final_url"`
	StatusCode   int                 `json:"status_code"`
	StatusText   string              `json:"status_text"`
	Headers      map[string][]string `json:"headers"`
	Redirects    []RedirectHop       `json:"redirect_chain,omitempty"`
	Timings      FetchTimings        `json:"timings"`
	Body         []byte              `json:"-"`
	BodySize     int                 `json:"body_size_bytes"`
	ContentType  string              `json:"content_type"`
}

// ClientOptions configures safe network behavior
type ClientOptions struct {
	UserAgent       string
	Timeout         time.Duration
	MaxRedirects    int
	MaxBodyBytes    int64
	AllowPrivateIPs bool // Set true only in testing environments
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
	if opts.Timeout == 0 {
		opts.Timeout = 15 * time.Second
	}
	if opts.MaxRedirects == 0 {
		opts.MaxRedirects = 10
	}
	if opts.MaxBodyBytes == 0 {
		opts.MaxBodyBytes = 15 * 1024 * 1024 // 15MB cap
	}

	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}

			if !opts.AllowPrivateIPs {
				ips, err := net.LookupIP(host)
				if err != nil {
					return nil, fmt.Errorf("DNS lookup failed: %w", err)
				}
				for _, ip := range ips {
					if isBlockedIP(ip) {
						return nil, fmt.Errorf("%w: %s (%s)", ErrSSRFBlocked, host, ip)
					}
				}
			}

			return dialer.DialContext(ctx, network, net.JoinHostPort(host, port))
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
		},
		options: opts,
	}
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
		redirects []RedirectHop
		currentURL = targetURL
	)

	// Step-by-step redirect follower to accurately capture every hop's headers and status
	for hopCount := 0; hopCount <= c.options.MaxRedirects; hopCount++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, currentURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", c.options.UserAgent)
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9,ja;q=0.8")

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

		// Disable automatic redirect following on the transport client for manual inspection
		c.httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}

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
			})

			currentURL = nextURL.String()
			parsed = nextURL
			continue
		}

		// Terminal response
		defer resp.Body.Close()
		limitedReader := io.LimitReader(resp.Body, c.options.MaxBodyBytes)
		bodyBytes, err := io.ReadAll(limitedReader)
		if err != nil {
			return nil, fmt.Errorf("failed reading body: %w", err)
		}

		if resp.Uncompressed && len(resp.Header["Content-Encoding"]) == 0 {
			resp.Header["Content-Encoding"] = []string{"gzip"}
		}

		return &FetchResult{

			RequestedURL: targetURL,
			FinalURL:     currentURL,
			StatusCode:   resp.StatusCode,
			StatusText:   http.StatusText(resp.StatusCode),
			Headers:      resp.Header,
			Redirects:    redirects,
			Timings:      timings,
			Body:         bodyBytes,
			BodySize:     len(bodyBytes),
			ContentType:  strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]),
		}, nil
	}

	return nil, ErrTooManyRedirects
}
