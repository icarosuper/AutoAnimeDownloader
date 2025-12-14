package api

import (
	"encoding/json"
	"net/http"
)

// SuccessResponse é a estrutura padrão de resposta de sucesso
type SuccessResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
	Error   *ErrorInfo  `json:"error"`
}

// ErrorInfo contém informações sobre erros
type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// JSONSuccess envia uma resposta JSON de sucesso
func JSONSuccess(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	
	response := SuccessResponse{
		Success: true,
		Data:    data,
		Error:   nil,
	}
	
	if err := json.NewEncoder(w).Encode(response); err != nil {
		// Se falhar ao codificar JSON, logar erro (mas não podemos usar logger aqui sem importar)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// JSONError envia uma resposta JSON de erro
func JSONError(w http.ResponseWriter, statusCode int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	
	response := SuccessResponse{
		Success: false,
		Data:    nil,
		Error: &ErrorInfo{
			Code:    code,
			Message: message,
		},
	}
	
	if err := json.NewEncoder(w).Encode(response); err != nil {
		// Se falhar ao codificar JSON, logar erro
		http.Error(w, "Failed to encode error response", http.StatusInternalServerError)
	}
}

// JSONInternalError envia uma resposta JSON de erro interno (500)
func JSONInternalError(w http.ResponseWriter, err error) {
	JSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
}

