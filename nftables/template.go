package nftables

import (
	"bytes"
	"html/template"
	"net/netip"
)

func renderNftablesTemplate(sshdPort uint16, v4Prefixes, v6Prefixes []netip.Prefix) (string, error) {
	configTemplate := `
{{- $v4Prefixes := .IPv4Prefixes -}}
{{- $v6Prefixes := .IPv6Prefixes -}}
#!/usr/sbin/nft -f

flush ruleset

table inet filter {
	{{- if ne (len $v4Prefixes) 0 }}
	set allowed_ipv4 {
		type ipv4_addr; flags interval; auto-merge;
		elements = {
			{{- range $index, $item := $v4Prefixes }}
			{{ $item }}{{ if lt $index (sub (len $v4Prefixes) 1) }},{{ end }}
			{{- end }}
		}
	}
	{{- end }}

	{{- if ne (len $v6Prefixes) 0 }}
	set allowed_ipv6 {
		type ipv6_addr; flags interval; auto-merge;
		elements = {
			{{- range $index, $item := $v6Prefixes }}
			{{ $item }}{{ if lt $index (sub (len $v6Prefixes) 1) }},{{ end }}
			{{- end }}
		}
	}
	{{- end }}

	chain input {
		type filter hook input priority filter; policy drop;
		ct state established,related accept
		iif lo accept
		tcp dport {{ .SSHDPort }} accept

		{{- if ne (len $v4Prefixes) 0 }}
		ip saddr @allowed_ipv4 accept
		{{- end }}

		{{- if ne (len $v6Prefixes) 0 }}
		ip6 saddr @allowed_ipv6 accept
		{{- end }}
	}

	chain forward {
		type filter hook forward priority filter; policy drop;
	}

	chain output {
		type filter hook output priority filter; policy accept;
	}
}
`

	type NftConf struct {
		SSHDPort     uint16
		IPv4Prefixes []netip.Prefix
		IPv6Prefixes []netip.Prefix
	}
	funcMap := template.FuncMap{
		"sub": func(a, b int) int { return a - b },
		"len": func(v interface{}) int { return len(v.([]netip.Prefix)) },
	}
	t := template.Must(template.New("nftables.conf").Funcs(funcMap).Parse(configTemplate))

	var result bytes.Buffer
	err := t.Execute(&result, NftConf{
		SSHDPort:     sshdPort,
		IPv4Prefixes: v4Prefixes,
		IPv6Prefixes: v6Prefixes,
	})
	if err != nil {
		return "", err
	}

	return result.String(), nil
}
