package drivers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"go-db-viewer/internal/core"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func TestMongoCursorLimitPreservesBSONAndClosesCursor(t *testing.T) {
	amount, err := bson.ParseDecimal128("12345678901234567890.123456789")
	if err != nil {
		t.Fatal(err)
	}
	document := bson.D{{Key: "_id", Value: int64(9223372036854775807)}, {Key: "amount", Value: amount}, {Key: "nested", Value: bson.D{{Key: "name", Value: "ação"}}}, {Key: "nullable", Value: nil}}
	cursor, err := mongo.NewCursorFromDocuments([]any{document, document}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	r, err := mongoDocuments(t.Context(), cursor, 1)
	if err != nil || len(r.Documents) != 1 || !r.HasMore || !r.Truncated {
		t.Fatal("limite do cursor incorreto")
	}
	for _, value := range []string{"$numberLong", "9223372036854775807", "$numberDecimal", "12345678901234567890.123456789", "ação", "null"} {
		if !strings.Contains(r.Documents[0], value) {
			t.Fatalf("resultado não preservou %s", value)
		}
	}
	if cursor.Next(t.Context()) {
		t.Fatal("cursor permaneceu aberto")
	}
}

func TestMongoDocumentTruncationPreservesUTF8(t *testing.T) {
	raw, err := bson.Marshal(bson.D{{Key: "value", Value: strings.Repeat("á", core.MaxCellBytes)}})
	if err != nil {
		t.Fatal(err)
	}
	text, truncated, err := mongoDocument(raw)
	if err != nil || !truncated || !utf8.ValidString(text) || !strings.HasSuffix(text, "[Documento truncado]") {
		t.Fatal("truncamento inválido")
	}
}

func TestMongoConnectionErrorsAreActionableAndHideSecrets(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"timeout", fmt.Errorf("mongodb://user:secret@host: %w", context.DeadlineExceeded), "não respondeu"},
		{"auth", fmt.Errorf("wrapped: %w", mongo.CommandError{Code: 18, Message: "secret"}), "authSource"},
		{"permission", mongo.CommandError{Code: 13, Message: "secret"}, "permissão"},
		{"network", mongo.CommandError{Labels: []string{"NetworkError"}, Message: "secret"}, "rede"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := mongoConnectionError(tc.err)
			if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "secret") {
				t.Fatalf("mensagem: %v", err)
			}
			if _, ok := errors.AsType[core.SafeFault](err); !ok {
				t.Fatal("erro de conexão sem mensagem controlada")
			}
		})
	}
	if mongoConnectionError(nil) != nil {
		t.Fatal("falha em conexão válida")
	}
	if !errors.Is(mongoConnectionError(context.Canceled), context.Canceled) {
		t.Fatal("cancelamento perdido")
	}
}
