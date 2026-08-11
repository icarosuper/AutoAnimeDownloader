package daemon

import (
	"net"
	"testing"
	"time"
)

// TestWebUIURL_StripsTheAddrColon: getPort()/tm.apiPort carregam o Addr do http.Server
// (":8091"), nao a porta nua. Interpolar isso direto produz "http://localhost::8091" — host
// invalido, e o navegador abre numa pagina de erro em vez do app. Foi exatamente o que o boot
// de primeira execucao fez.
func TestWebUIURL_StripsTheAddrColon(t *testing.T) {
	cases := []struct {
		port string
		want string
	}{
		{":8091", "http://localhost:8091/#/status"},
		{"8091", "http://localhost:8091/#/status"},
		{"", "http://localhost:8091/#/status"},
		{":", "http://localhost:8091/#/status"},
	}

	for _, tc := range cases {
		if got := WebUIURL(tc.port, "/#/status"); got != tc.want {
			t.Errorf("WebUIURL(%q) = %q, quero %q", tc.port, got, tc.want)
		}
	}
}

func TestWebUIURL_EmptyRouteIsTheRoot(t *testing.T) {
	if got := WebUIURL(":8091", ""); got != "http://localhost:8091" {
		t.Fatalf("quero a raiz sem sufixo, veio %q", got)
	}
}

// TestWaitForListener_ReturnsOnceBound: o servidor da API sobe numa goroutine, entao abrir o
// navegador logo depois e uma corrida com o ListenAndServe — perder a corrida da
// ERR_CONNECTION_REFUSED, o mesmo sintoma visivel de "abriu sem o app".
func TestWaitForListener_ReturnsOnceBound(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	if err := WaitForListener(ln.Addr().String(), time.Second); err != nil {
		t.Fatalf("porta ja escutando deve voltar sem erro: %v", err)
	}
}

func TestWaitForListener_TimesOut(t *testing.T) {
	// Porta fechada: reserva uma e libera, para ter um numero que ninguem esta escutando.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	if err := WaitForListener(addr, 150*time.Millisecond); err == nil {
		t.Fatal("quero erro quando ninguem sobe na porta")
	}
}
