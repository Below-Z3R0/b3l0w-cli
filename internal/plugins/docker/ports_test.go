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

func TestParsePortsIPv6(t *testing.T) {
	// El binding IPv6 viene como "[::]:8080->80/tcp": la IP conserva los
	// dos puntos pero pierde los corchetes.
	ports := ParsePorts("[::]:8080->80/tcp")
	if len(ports) != 1 {
		t.Fatalf("esperaba 1 puerto, obtuve %d: %+v", len(ports), ports)
	}
	if ports[0].HostIP != "::" {
		t.Errorf("HostIP = %q, quería ::", ports[0].HostIP)
	}
	if ports[0].HostPort != 8080 || ports[0].Container != 80 {
		t.Errorf("puertos mal: %+v", ports[0])
	}
	if ports[0].Proto != "tcp" {
		t.Errorf("Proto = %q", ports[0].Proto)
	}
}

func TestParsePortsIPv6Espefico(t *testing.T) {
	// Un binding a una IPv6 concreta: la IP puede traer una barra.
	ports := ParsePorts("[fe80::1]:8080->80/tcp")
	if len(ports) != 1 {
		t.Fatalf("esperaba 1 puerto, obtuve %d", len(ports))
	}
	if ports[0].HostIP != "fe80::1" {
		t.Errorf("HostIP = %q, quería fe80::1", ports[0].HostIP)
	}
}

func TestParsePortsIPv4YIPv6NoSeDuplican(t *testing.T) {
	// El mismo puerto publicado en las dos familias: es un solo puerto.
	ports := ParsePorts("0.0.0.0:8080->80/tcp, [::]:8080->80/tcp")
	if len(ports) != 1 {
		t.Fatalf("esperaba 1 puerto, obtuve %d: %+v", len(ports), ports)
	}
	// Gana el primero que apareció.
	if ports[0].HostIP != "0.0.0.0" {
		t.Errorf("HostIP = %q, quería 0.0.0.0", ports[0].HostIP)
	}
}

func TestParsePortsUDP(t *testing.T) {
	ports := ParsePorts("0.0.0.0:53->53/udp")
	if len(ports) != 1 {
		t.Fatalf("esperaba 1 puerto, obtuve %d", len(ports))
	}
	if ports[0].Proto != "udp" {
		t.Errorf("Proto = %q, quería udp", ports[0].Proto)
	}
}

func TestParsePortsOrdenaPorPuertoDeHost(t *testing.T) {
	ports := ParsePorts("0.0.0.0:9000->90/tcp, 0.0.0.0:80->80/tcp, 0.0.0.0:443->443/tcp")
	if len(ports) != 3 {
		t.Fatalf("esperaba 3, obtuve %d", len(ports))
	}
	if ports[0].HostPort != 80 || ports[1].HostPort != 443 || ports[2].HostPort != 9000 {
		t.Errorf("orden = %d, %d, %d", ports[0].HostPort, ports[1].HostPort, ports[2].HostPort)
	}
}

func TestParsePortsSalidaVacia(t *testing.T) {
	if ports := ParsePorts(""); ports != nil {
		t.Errorf("sin puertos debería quedar nil, obtuve %+v", ports)
	}
	if ports := ParsePorts("   "); ports != nil {
		t.Errorf("sin puertos debería quedar nil, obtuve %+v", ports)
	}
}

func TestParsePortsDescartaLoRoto(t *testing.T) {
	// Un puerto sin número de contenedor no sirve: se descarta.
	ports := ParsePorts("0.0.0.0:8080->/tcp, 0.0.0.0:8081->80/tcp")
	if len(ports) != 1 {
		t.Fatalf("esperaba 1 puerto, obtuve %d: %+v", len(ports), ports)
	}
	if ports[0].HostPort != 8081 {
		t.Errorf("HostPort = %d, quería 8081", ports[0].HostPort)
	}
}

func TestParsePortsSoloInternoNoTieneURL(t *testing.T) {
	// Un puerto publicado en el contenedor pero no en el host no se abre
	// desde el navegador: no lleva URL.
	ports := ParsePorts("80/tcp")
	if len(ports) != 1 {
		t.Fatalf("esperaba 1, obtuve %d", len(ports))
	}
	if ports[0].URL != "" {
		t.Errorf("URL = %q, quería vacío", ports[0].URL)
	}
	if ports[0].HostPort != 0 {
		t.Errorf("HostPort = %d, quería 0", ports[0].HostPort)
	}
}
