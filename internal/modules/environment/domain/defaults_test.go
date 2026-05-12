package domain

import (
	"testing"
)

func TestIsPrivateOrLocalIP(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		wantPrivate bool
	}{
		{
			name:    "localhost IPv4",
			ip:      "127.0.0.1",
			wantPrivate: true,
		},
		{
			name:    "localhost IPv6",
			ip:      "::1",
			wantPrivate: true,
		},
		{
			name:    "rango privado 192.168.x.x",
			ip:      "192.168.1.1",
			wantPrivate: true,
		},
		{
			name:    "rango privado 10.x.x.x",
			ip:      "10.0.0.1",
			wantPrivate: true,
		},
		{
			name:    "rango privado 172.16-31.x.x",
			ip:      "172.16.0.1",
			wantPrivate: true,
		},
		{
			name:    "IP pública 8.8.8.8",
			ip:      "8.8.8.8",
			wantPrivate: false,
		},
		{
			name:    "IP pública 1.1.1.1",
			ip:      "1.1.1.1",
			wantPrivate: false,
		},
		{
			name:    "string vacío",
			ip:      "",
			wantPrivate: true,
		},
		{
			name:    "IP malformada",
			ip:      "not-an-ip",
			wantPrivate: true,
		},
		{
			name:    "rango privado 172.31.255.255",
			ip:      "172.31.255.255",
			wantPrivate: true,
		},
		{
			name:    "fuera de rango 172.x 172.32.0.1",
			ip:      "172.32.0.1",
			wantPrivate: false,
		},
		{
			name:    "IPv6 pública",
			ip:      "2001:4860:4860::8888",
			wantPrivate: false,
		},
		{
			name:    "IPv6 loopback completa",
			ip:      "0:0:0:0:0:0:0:1",
			wantPrivate: true,
		},
		{
			name:    "IPv4 no especificada 0.0.0.0",
			ip:      "0.0.0.0",
			wantPrivate: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IsPrivateOrLocalIP(tc.ip)
			if got != tc.wantPrivate {
				t.Errorf("IsPrivateOrLocalIP(%q) = %v, esperaba %v", tc.ip, got, tc.wantPrivate)
			}
		})
	}
}

func TestIsPrivateOrLocalIP_UsadaPorHandler(t *testing.T) {
	// Simula el flujo del handler: IP privada → 400.
	ipPrivada := "192.168.0.1"
	if !IsPrivateOrLocalIP(ipPrivada) {
		t.Errorf("una IP privada %q debe ser detectada para rechazar en el handler", ipPrivada)
	}

	// Simula el flujo del handler: IP pública → proceder.
	ipPublica := "8.8.8.8"
	if IsPrivateOrLocalIP(ipPublica) {
		t.Errorf("una IP pública %q NO debe ser rechazada por el handler", ipPublica)
	}
}
