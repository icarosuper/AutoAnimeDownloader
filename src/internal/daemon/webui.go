package daemon

import (
	"fmt"
	"net"
	"strings"
	"time"
)

const defaultAPIPort = "8091"

// WebUIURL monta a URL da Web UI a partir do que o processo chama de "porta", que na pratica e
// o Addr do http.Server: ":8091", com dois-pontos na frente. Interpolar esse valor direto num
// "http://localhost:%s" produz "http://localhost::8091" — host invalido, e o navegador abre
// numa pagina de erro em vez do app.
//
// Existe porque essa conversao ja estava escrita em tres lugares (o passe de verificacao, o
// tray e o boot de primeira execucao) e o mais novo esqueceu do TrimPrefix. Uma funcao so.
//
// route entra como veio ("/#/status", "/api/v1/check", "" para a raiz).
func WebUIURL(port, route string) string {
	port = strings.TrimPrefix(port, ":")
	if port == "" {
		port = defaultAPIPort
	}
	return fmt.Sprintf("http://localhost:%s%s", port, route)
}

// WaitForListener espera a porta aceitar conexao, ate timeout.
//
// O servidor da API sobe numa goroutine e o log "API server started successfully" sai antes de
// o socket estar ligado. Quem abre o navegador logo depois disputa uma corrida com o
// ListenAndServe: perder da ERR_CONNECTION_REFUSED, que para o usuario e o mesmo "abriu sem o
// app" de uma URL quebrada.
func WaitForListener(addr string, timeout time.Duration) error {
	// O Addr do http.Server costuma vir sem host (":8091"); net.Dial precisa de um.
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}

	deadline := time.Now().Add(timeout)
	for {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s to accept connections: %w", addr, err)
		}
		time.Sleep(25 * time.Millisecond)
	}
}
