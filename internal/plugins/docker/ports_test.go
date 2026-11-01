package docker

import "testing"

// El caso real del plugin original: el mismo puerto publicado en IPv4 e IPv6, y
// el interno repetido por tcp/tcp6. Mostraba chips duplicados.
func TestParsePortsDedupe(t *testing.T) {
	raw := "0.0.0.0:43211->43211/tcp, [::]:43211->43211/tcp, " +
		"0.0.0.0:43213->43213/tcp, 0.0.0.0:43213->43213/tcp"
	ports := ParsePorts(raw)
	if len(ports) != 2 {
		t.Fatalf("esperaba 2 puertos, obtuve %d: %+v", len(ports), ports)
	}
	if ports[0].HostPort != 43211 || ports[0].URL != "http://localhost:43211" {
		t.Fatalf("primer puerto mal: %+v", ports[0])
	}
}

func TestParsePortsOnlyInternal(t *testing.T) {
	ports := ParsePorts("80/tcp, 443/tcp")
	if len(ports) != 2 {
		t.Fatalf("esperaba 2, obtuve %d", len(ports))
	}
	if ports[0].HostPort != 0 || ports[0].URL != "" {
		t.Fatalf("un puerto sin publicar no tiene URL: %+v", ports[0])
	}
}

func TestParsePortsBindingIP(t *testing.T) {
	ports := ParsePorts("127.0.0.1:8080->80/tcp")
	if len(ports) != 1 || ports[0].HostIP != "127.0.0.1" || ports[0].Container != 80 {
		t.Fatalf("binding mal parseado: %+v", ports)
	}
}

func TestPortValido(t *testing.T) {
	bien := []string{"8080:80", "80", "127.0.0.1:8080:80/tcp"}
	mal := []string{"", "abc", "8080:", "8080:abc"}
	for _, v := range bien {
		if !puertoValido(v) {
			t.Errorf("%q debería ser válido", v)
		}
	}
	for _, v := range mal {
		if puertoValido(v) {
			t.Errorf("%q debería ser inválido", v)
		}
	}
}
