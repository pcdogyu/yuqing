package jin10full

import (
	"context"
	"net/url"
	"strings"
)

type robotsRules map[string][]string

func (p *Provider) loadRobots(ctx context.Context) robotsRules {
	rules := robotsRules{}
	for host, raw := range robotsURLs(p.opts) {
		resp, err := p.client.R().SetContext(ctx).Get(raw)
		if err != nil || resp.IsError() {
			continue
		}
		rules[host] = parseDisallow(resp.String())
		p.pause(ctx)
	}
	return rules
}

func (r robotsRules) Allowed(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return false
	}
	for _, path := range r[parsed.Host] {
		if path == "/" {
			return false
		}
		if path != "" && strings.HasPrefix(parsed.Path, path) {
			return false
		}
	}
	return true
}

func parseDisallow(body string) []string {
	lines := strings.Split(body, "\n")
	out := make([]string, 0)
	inWildcardAgent := false
	for _, line := range lines {
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = line[:idx]
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "user-agent":
			inWildcardAgent = value == "*" || value == ""
		case "disallow":
			if inWildcardAgent && value != "" {
				out = append(out, value)
			}
		}
	}
	return out
}

func robotsURLs(opts Options) map[string]string {
	out := map[string]string{
		"www.jin10.com":   "https://www.jin10.com/robots.txt",
		"flash.jin10.com": "https://flash.jin10.com/robots.txt",
		"xnews.jin10.com": "https://xnews.jin10.com/robots.txt",
	}
	for _, raw := range []string{opts.FlashURL, opts.HeadlineURL} {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			continue
		}
		parsed.Path = "/robots.txt"
		parsed.RawQuery = ""
		parsed.Fragment = ""
		out[parsed.Host] = parsed.String()
	}
	return out
}
