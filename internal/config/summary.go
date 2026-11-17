package config

import (
	"fmt"
	"net/url"
)

// Summary is the minimal 01a output; the full effective-config summary is
// owned by 01b.
func (b *Bundle) Summary() string {
	return fmt.Sprintf(
		"config ok: node=%s product_key=%s links=%d devices=%d profiles=%d",
		b.Config.Node.ID,
		b.Config.Node.ProductKey,
		len(b.Links),
		len(b.Devices),
		len(b.Profiles),
	)
}

// SecretValues returns every resolved credential plus broker userinfo
// passwords, for the logging redactor's value-level fallback.
func (b *Bundle) SecretValues() []string {
	var out []string
	add := func(s Secret) {
		if s.value != "" {
			out = append(out, s.value)
		}
	}
	add(b.Config.Northbound.MQTT.Password)
	for i := range b.Links {
		if b.Links[i].Chirpstack != nil {
			add(b.Links[i].Chirpstack.apiToken)
		}
	}

	urls := []string{b.Config.Northbound.MQTT.Broker}
	for i := range b.Links {
		if b.Links[i].Chirpstack != nil {
			urls = append(urls, b.Links[i].Chirpstack.MQTT.Broker)
		}
	}
	for _, raw := range urls {
		u, err := url.Parse(raw)
		if err != nil || u.User == nil {
			continue
		}
		if p, ok := u.User.Password(); ok && p != "" {
			out = append(out, p)
		}
	}
	return out
}
