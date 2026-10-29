package drivers

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"go-db-viewer/internal/core"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type MongoDriver struct{}

func (s *mongoSession) ListTables(context.Context) ([]core.TableSuggestion, error) {
	return nil, core.SafeFault("O autocomplete de tabelas está disponível somente no editor SQL.")
}

func (MongoDriver) Info() core.DriverInfo {
	return core.DriverInfo{Kind: core.MongoDB, Name: "MongoDB", Language: "json", Example: "mongodb://usuario:senha@localhost:27017/banco?authSource=admin"}
}
func (MongoDriver) Validate(dsn string) error {
	u, err := url.Parse(dsn)
	if err != nil || u.Host == "" || (u.Scheme != "mongodb" && u.Scheme != "mongodb+srv") || len(dsn) > 65536 {
		return errors.New("URI MongoDB inválida. Use mongodb:// ou mongodb+srv://.")
	}
	return nil
}
func (MongoDriver) Open(ctx context.Context, dsn string) (core.Session, error) {
	c, err := mongo.Connect(options.Client().ApplyURI(dsn).SetRetryReads(false).SetRetryWrites(false).SetServerSelectionTimeout(15 * time.Second))
	if err != nil {
		return nil, err
	}
	return &mongoSession{client: c}, nil
}

type mongoSession struct{ client *mongo.Client }

func (s *mongoSession) Ping(ctx context.Context) error {
	return mongoConnectionError(s.client.Ping(ctx, nil))
}

// As mensagens da conexão não incluem a URI nem as credenciais.
func mongoConnectionError(err error) error {
	if err == nil || errors.Is(err, context.Canceled) {
		return err
	}
	if server, ok := errors.AsType[mongo.ServerError](err); ok {
		if server.HasErrorCode(18) {
			return core.SafeFault("Autenticação MongoDB recusada. Verifique usuário, senha e authSource.")
		}
		if server.HasErrorCode(13) {
			return core.SafeFault("O usuário MongoDB não tem permissão para testar esta conexão.")
		}
	}
	if mongo.IsTimeout(err) {
		return core.SafeFault("O MongoDB não respondeu ao teste de conexão no prazo. Verifique endereço, porta, TLS e o estado do serviço. No Compose local, use 127.0.0.1 e aguarde o estado healthy.")
	}
	if mongo.IsNetworkError(err) {
		return core.SafeFault("Falha de rede ao conectar ao MongoDB. Verifique endereço, porta, TLS e o estado do serviço.")
	}
	return err
}
func (s *mongoSession) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.client.Disconnect(ctx)
}
func (s *mongoSession) List(ctx context.Context, parent core.Node) ([]core.Node, error) {
	var names []string
	var err error
	kind := "database"
	if parent.Kind == "" {
		names, err = s.client.ListDatabaseNames(ctx, bson.D{})
	} else if parent.Kind == "database" {
		names, err = s.client.Database(parent.Name).ListCollectionNames(ctx, bson.D{})
		kind = "collection"
	} else {
		return []core.Node{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]core.Node, 0, len(names))
	for _, name := range names {
		n := core.Node{Name: name, Kind: kind, Database: parent.Database}
		if kind == "database" {
			n.Database = name
		}
		out = append(out, n)
	}
	return out, nil
}
func (s *mongoSession) Describe(ctx context.Context, n core.Node) ([]core.Column, error) {
	var raw bson.Raw
	err := s.client.Database(n.Database).Collection(n.Name).FindOne(ctx, bson.D{}).Decode(&raw)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return []core.Column{}, nil
	}
	if err != nil {
		return nil, err
	}
	elements, err := raw.Elements()
	if err != nil {
		return nil, err
	}
	out := make([]core.Column, 0, len(elements))
	for _, e := range elements {
		out = append(out, core.Column{Name: e.Key(), Type: e.Value().Type.String()})
	}
	return out, nil
}
func (s *mongoSession) Preview(ctx context.Context, req core.PreviewRequest) (core.Result, error) {
	limit, err := core.ResolveRowLimit(req.RowLimit, core.DefaultRowLimit)
	if err != nil {
		return core.Result{}, err
	}
	if req.Node.Kind != "collection" {
		return core.Result{}, core.SafeFault("Selecione uma coleção.")
	}
	cursor, err := s.client.Database(req.Node.Database).Collection(req.Node.Name).Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetSkip(int64(req.Page)*int64(limit)).SetLimit(int64(limit+1)).SetBatchSize(int32(limit+1)))
	if err != nil {
		return core.Result{}, err
	}
	return mongoDocuments(ctx, cursor, limit)
}
func (s *mongoSession) Execute(ctx context.Context, req core.Request) (core.Result, error) {
	limit, err := core.ResolveRowLimit(req.RowLimit, core.DefaultRowLimit)
	if err != nil {
		return core.Result{}, err
	}
	if strings.TrimSpace(req.Database) == "" {
		return core.Result{}, core.SafeFault("Selecione um banco MongoDB.")
	}
	var command bson.D
	if bson.UnmarshalExtJSON([]byte(req.Text), false, &command) != nil || len(command) == 0 {
		return core.Result{}, core.SafeFault("Comando Extended JSON inválido. Informe um objeto JSON.")
	}
	for _, field := range command {
		switch field.Key {
		case "lsid", "txnNumber", "autocommit", "startTransaction", "$db":
			return core.Result{}, core.SafeFault("Campos de sessão, transação e $db não são aceitos. Selecione o banco no editor.")
		}
	}
	name := strings.ToLower(command[0].Key)
	db := s.client.Database(req.Database)
	switch name {
	case "find", "aggregate", "listcollections", "listindexes":
		cursor, err := db.RunCommandCursor(ctx, command)
		if err != nil {
			return core.Result{}, err
		}
		return mongoDocuments(ctx, cursor, limit)
	case "insert", "update", "delete", "findandmodify", "count", "distinct", "create", "drop", "createindexes", "dropindexes", "collstats", "dbstats", "ping":
		var raw bson.Raw
		if err := db.RunCommand(ctx, command).Decode(&raw); err != nil {
			return core.Result{}, err
		}
		// RunCommand retorna erros de escrita dentro da resposta BSON.
		if v := raw.Lookup("writeErrors"); v.Type != 0 {
			array, ok := v.ArrayOK()
			if !ok {
				return core.Result{}, errors.New("Resposta MongoDB inválida.")
			}
			values, err := array.Values()
			if err != nil {
				return core.Result{}, err
			}
			if len(values) > 0 {
				return core.Result{}, core.SafeFault("O MongoDB retornou erros de escrita. Parte do comando pode ter sido aplicada; confira os dados.")
			}
		}
		if raw.Lookup("writeConcernError").Type != 0 {
			return core.Result{}, core.SafeFault("O MongoDB não confirmou o write concern. Confira os dados antes de repetir.")
		}
		text, truncated, err := mongoDocument(raw)
		if err != nil {
			return core.Result{}, err
		}
		return core.Result{Documents: []string{text}, Truncated: truncated, Message: "Comando concluído."}, nil
	default:
		return core.Result{}, core.SafeFault("Comando MongoDB não suportado. Consulte os comandos disponíveis no README.")
	}
}
func mongoDocuments(ctx context.Context, cursor *mongo.Cursor, limit int) (core.Result, error) {
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = cursor.Close(closeCtx)
	}()
	r := core.Result{Documents: []string{}, RowLimit: limit}
	size := 0
	for cursor.Next(ctx) {
		if len(r.Documents) >= limit {
			r.Truncated = true
			r.HasMore = true
			r.LimitReason = "rows"
			break
		}
		text, truncated, err := mongoDocument(cursor.Current)
		if err != nil {
			return core.Result{}, err
		}
		r.Truncated = r.Truncated || truncated
		if size+2*len(text)+64 > core.MaxResultBytes {
			r.Truncated = true
			r.LimitReason = "bytes"
			r.Message = "Limite de tamanho atingido. Use projeção ou filtros para reduzir os documentos."
			break
		}
		r.Documents = append(r.Documents, text)
		size += 2*len(text) + 64
	}
	return r, cursor.Err()
}

func mongoDocument(raw bson.Raw) (string, bool, error) {
	b, err := bson.MarshalExtJSON(raw, true, false)
	if err != nil {
		return "", false, err
	}
	text := string(b)
	if len(text) <= core.MaxCellBytes {
		return text, false, nil
	}
	text = text[:core.MaxCellBytes]
	for !utf8.ValidString(text) {
		text = text[:len(text)-1]
	}
	return text + "\n[Documento truncado]", true, nil
}

func (s *mongoSession) ResolveTable(context.Context, core.TableReference) (core.Node, error) {
	return core.Node{}, core.SafeFault("Esta operação está disponível apenas para bancos SQL.")
}
func (s *mongoSession) GetTableProperties(context.Context, core.Node) (core.TableProperties, error) {
	return core.TableProperties{}, core.SafeFault("Esta operação está disponível apenas para bancos SQL.")
}
