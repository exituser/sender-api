package validator

import (
	"net"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
)

var (
	emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	slugRegex  = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
)

func IsValidEmail(email string) bool {
	if email == "" || strings.ContainsAny(email, "\r\n") {
		return false
	}
	address, err := mail.ParseAddress(email)
	return err == nil && address.Address == email && emailRegex.MatchString(email)
}

func EmailDomain(email string) string {
	address, err := mail.ParseAddress(strings.TrimSpace(email))
	if err != nil {
		return ""
	}
	separator := strings.LastIndex(address.Address, "@")
	if separator < 0 || separator == len(address.Address)-1 {
		return ""
	}
	return strings.ToLower(strings.TrimSuffix(address.Address[separator+1:], "."))
}

func CanonicalEmail(email string) string {
	address, err := mail.ParseAddress(strings.TrimSpace(email))
	if err != nil {
		return strings.TrimSpace(email)
	}
	separator := strings.LastIndex(address.Address, "@")
	if separator < 0 || separator == len(address.Address)-1 {
		return address.Address
	}
	return address.Address[:separator] + "@" + strings.ToLower(address.Address[separator+1:])
}

func IsValidSlug(slug string) bool {
	return slugRegex.MatchString(slug)
}

func SanitizeString(s string) string {
	return strings.TrimSpace(s)
}

func IsValidURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil || len(rawURL) > 2048 || (strings.ToLower(parsed.Scheme) != "http" && strings.ToLower(parsed.Scheme) != "https") || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return false
	}
	return !IsPrivateHost(parsed.Hostname())
}

func IsValidHTTPSURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	return err == nil && strings.EqualFold(parsed.Scheme, "https") && IsValidURL(rawURL)
}

func IsValidDomain(domain string) bool {
	domain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
	if domain == "" || net.ParseIP(domain) != nil {
		return false
	}
	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if part == "" || len(part) > 63 || part[0] == '-' || part[len(part)-1] == '-' {
			return false
		}
		for _, char := range part {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
				return false
			}
		}
	}
	return len(parts[len(parts)-1]) >= 2
}

func ContainsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func IsPrivateHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return IsPrivateIP(ip)
}

func IsPrivateIP(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 0 {
			return true
		}
		ip = ip4
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}
