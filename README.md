# Go DB Viewer

Aplicativo desktop com Go, Wails v2 e Vue 3 para explorar bancos e executar comandos de leitura e escrita.

O visual segue o AvocadoTerm: destaque verde-água, fonte Nunito local e temas claro, escuro ou conforme o sistema.

## Bancos e conexões

| Banco | Driver | Formato |
|---|---|---|
| PostgreSQL | pgx/v5 | `postgresql://usuario:senha@localhost:5432/banco?sslmode=require` |
| Oracle | go-ora/v2 | `oracle://usuario:senha@localhost:1521/servico` |
| SQL Server | go-mssqldb | `sqlserver://usuario:senha@localhost:1433?database=banco&encrypt=true` |
| MongoDB | mongo-driver/v2 | `mongodb://usuario:senha@localhost:27017/banco?authSource=admin` |
| Ignite 2 | ODBC oficial | `DRIVER={Apache Ignite};ADDRESS=localhost:10800;SCHEMA=PUBLIC;USER=usuario;PASSWORD=senha;` |
| Ignite 3 | ODBC oficial | `DRIVER={Apache Ignite 3};ADDRESS=localhost:10800;SCHEMA=PUBLIC;IDENTITY=usuario;SECRET=senha;` |

Use as opções TLS e autenticação do driver. Para URLs, codifique caracteres reservados nas credenciais.
Não use strings de exemplo com credenciais reais em arquivos versionados.

Os adaptadores SQL exploram o banco indicado na string. Para trocar esse banco, crie outra conexão.
MongoDB permite selecionar os bancos disponíveis para o usuário autenticado.

## Bancos locais com Docker Compose

O arquivo `compose.yaml` inicia PostgreSQL 17, MongoDB 7.0, Oracle Free 23 e SQL Server 2022 Developer.
As credenciais abaixo são exclusivas para desenvolvimento local. As portas ficam vinculadas a `127.0.0.1`.
O SQL Server Developer permite uso em desenvolvimento e testes, conforme a [documentação da Microsoft](https://learn.microsoft.com/en-us/sql/linux/quickstart-install-connect-docker?view=sql-server-ver16&pivots=cs1-bash).

Na raiz do projeto, execute:

```sh
docker compose up -d
docker compose ps -a
```

Aguarde os quatro bancos apresentarem estado `healthy`.
O serviço `sqlserver-init` deve terminar com código `0`. Ele cria o banco `dbviewer`, o login e as permissões.
A primeira inicialização do Oracle pode levar alguns minutos.

Para acompanhar a inicialização:

```sh
docker compose logs -f oracle sqlserver sqlserver-init
```

Se o MongoDB apresentar `Restarting` ou `Exited`, consulte o log antes de testar a conexão:

```sh
docker compose ps mongodb
docker compose logs --tail=80 mongodb
```

O estado `Restarting` indica falha no processo do container. Aumentar o timeout do aplicativo não resolve essa falha.
Após corrigir a causa indicada no log, execute `docker compose up -d mongodb` e aguarde o estado `healthy`.

### MongoDB e kernels Linux recentes

O Compose usa `mongo:7.0` para evitar a incompatibilidade do MongoDB 8 com determinados kernels Linux recentes.
A [matriz oficial de compatibilidade](https://jira.mongodb.org/browse/SERVER-125742) descreve o problema e as versões afetadas.
Docker compartilha o kernel do host. Trocar apenas a distribuição da imagem não resolve essa incompatibilidade.

Se o log informar `MongoDB cannot start` e mencionar o kernel `6.19`, aplique a configuração atual:

```sh
docker compose pull mongodb
docker compose up -d --force-recreate --wait --wait-timeout 120 mongodb
docker compose ps mongodb
```

O MongoDB 7 usa os volumes `mongodb7-data` e `mongodb7-config`.
Os volumes antigos `mongodb-data` e `mongodb-config`, usados pelo MongoDB 8, permanecem preservados e sem uso.
A nova instância começa vazia e cria o usuário `dbviewer` com as credenciais abaixo.
Não reutilize diretamente um volume do MongoDB 8 no MongoDB 7. Para transferir dados, use um procedimento de migração compatível.

### Dados de conexão para o Go DB Viewer

Use os dados abaixo quando o aplicativo executar no mesmo computador do Docker.

| Tipo no aplicativo | Host | Porta | Banco ou serviço | Usuário | Senha |
|---|---|---|---|---|---|
| PostgreSQL | `localhost` | `5432` | Banco `dbviewer` | `dbviewer` | `LocalDev2026` |
| MongoDB | `127.0.0.1` | `27017` | Banco `dbviewer`; `authSource=dbviewer` | `dbviewer` | `LocalDev2026` |
| Oracle | `localhost` | `1521` | Serviço `FREEPDB1`; schema `DBVIEWER` | `dbviewer` | `LocalDev2026` |
| SQL Server | `localhost` | `1433` | Banco `dbviewer`; schema `dbo` | `dbviewer` | `LocalDev2026` |

Copie a string correspondente para o campo **String de conexão completa**:

**PostgreSQL**

```text
postgresql://dbviewer:LocalDev2026@localhost:5432/dbviewer?sslmode=disable
```

**MongoDB**

```text
mongodb://dbviewer:LocalDev2026@127.0.0.1:27017/dbviewer?authSource=dbviewer
```

O script de inicialização cria o usuário no banco `dbviewer`, com a role `readWrite`.
Ele também cria a coleção vazia `connection_check`, para disponibilizar o banco na árvore de objetos.

**Oracle**

```text
oracle://dbviewer:LocalDev2026@localhost:1521/FREEPDB1
```

Use o serviço `FREEPDB1`. O usuário da aplicação pertence a esse PDB.

**SQL Server**

```text
sqlserver://dbviewer:LocalDev2026@localhost:1433?database=dbviewer&encrypt=true&TrustServerCertificate=true
```

Essa conexão usa TLS e confia no certificado local do container.
O usuário `dbviewer` recebe a role `db_owner` somente no banco `dbviewer`.

As contas administrativas locais são `admin` no MongoDB, `SYS`/`SYSTEM` no Oracle e `sa` no SQL Server.
A senha dessas contas é `LocalAdmin2026`. No MongoDB, a conta administrativa usa `authSource=admin`.

### Portas e persistência

Os bancos mantêm os dados em volumes Docker. Para interromper os serviços e preservar os dados:

```sh
docker compose down
```

Para remover os serviços **e apagar todos os dados locais**:

```sh
docker compose down -v
```

As imagens PostgreSQL, MongoDB e Oracle aplicam credenciais durante a primeira inicialização do volume.
Alterar as credenciais no Compose não atualiza usuários já existentes.
O script SQL Server preserva o banco e os usuários existentes quando executar novamente.

Se uma porta estiver ocupada, configure `POSTGRES_PORT`, `MONGODB_PORT`, `ORACLE_PORT` ou `SQLSERVER_PORT`.
Por exemplo:

```sh
POSTGRES_PORT=15432 docker compose up -d postgres
```

Nesse caso, use `15432` na string PostgreSQL. Para iniciar somente um banco, informe o nome do serviço.
Para SQL Server, inclua também a inicialização:

```sh
docker compose up -d sqlserver sqlserver-init
```

As configurações seguem as imagens [PostgreSQL](https://hub.docker.com/_/postgres),
[MongoDB](https://hub.docker.com/_/mongo), [Oracle Free](https://github.com/gvenzl/oci-oracle-free)
e [SQL Server](https://learn.microsoft.com/en-us/sql/linux/quickstart-install-connect-docker?view=sql-server-ver16&pivots=cs1-bash).

## Desenvolvimento

Requisitos: Go 1.27.2, Node.js 22.12 ou superior e Wails 2.15.0.

```sh
go mod tidy
go install github.com/wailsapp/wails/v2/cmd/wails@v2.15.0
cd frontend
npm ci
npm run build
cd ..
```

O primeiro `go mod tidy` resolve os módulos e gera `go.sum`. Ele exige acesso ao proxy Go.

### Builds e pacotes com Make

Na raiz do projeto, use os targets abaixo. Os artefatos ficam em `build/bin/`.

```sh
make build-exe     # Windows: .exe
make build-deb     # Debian/Ubuntu: .deb
make build-rpm     # Distribuições RPM: .rpm
make build-mac     # macOS: .app e .dmg
```

| Target | Sistema de build | Artefatos |
|---|---|---|
| `build-exe` | Linux ou macOS | `go-db-viewer-windows-amd64.exe` |
| `build-linux` | Linux | `go-db-viewer-linux-amd64` |
| `build-deb` | Linux | `go-db-viewer_0.1.0-1_amd64.deb` |
| `build-rpm` | Linux | `go-db-viewer-0.1.0-1.x86_64.rpm` |
| `build-mac` | macOS | `go-db-viewer.app` e `go-db-viewer-darwin-<arquitetura>.dmg` |

Os exemplos usam versão `0.1.0` e arquitetura `amd64`. Por padrão, `ARCH` usa a arquitetura nativa da máquina.
Para Windows ARM64, use `make build-exe ARCH=arm64`.
Para definir a versão dos pacotes Linux, use `make build-deb build-rpm VERSION=1.2.3`.
Na mesma chamada, o executável Linux é compilado uma única vez para os dois pacotes.

Requisitos comuns: GNU Make, Go e Node.js com npm. As versões do projeto estão na seção Desenvolvimento.
Os targets instalam Wails `v2.15.0` e nFPM `v2.48.0` sob demanda em `build/tools/`, sem alterar o `PATH`.
A primeira execução precisa de acesso à rede. Para instalar somente essas ferramentas, execute `make tools`.

No Linux, instale as dependências nativas indicadas abaixo. Os pacotes também exigem `readelf`, disponível no pacote `binutils`.
A compilação usa GTK3, WebKitGTK 4.1 e unixODBC. O pacote inclui o executável, o atalho no menu e este README.
As dependências de execução e a versão mínima da glibc exigida pelo executável entram nos metadados do pacote.
O mantenedor usa o autor do último commit Git. Para substituir, execute:

```sh
MAINTAINER='Seu nome <seu-email@exemplo.com>' make build-deb
```

Os targets Linux e macOS exigem a arquitetura nativa da máquina. Eles não fazem compilação cruzada entre arquiteturas.
Gere os pacotes Linux na distribuição-base mais antiga que pretende suportar.
Gerar um RPM em Debian/Ubuntu não valida sua execução em todas as distribuições RPM.
O sistema de destino precisa oferecer as bibliotecas GTK3, WebKitGTK 4.1 e unixODBC exigidas.

O `.exe` usa compilação cruzada e não exige compilador C para Windows.
Na máquina Windows de destino, instale WebView2 e os drivers ODBC do Ignite que forem necessários.
O executável Windows não foi validado em execução neste ambiente Linux.

Para `build-mac`, use macOS com Xcode Command Line Tools, Java 17 ou superior e Maven.
O target compila o processo JDBC, inclui os três JARs no bundle `.app` e gera um `.dmg`.
No macOS de destino, Java 17 ou superior continua necessário para usar Ignite via JDBC.
A imagem inclui um atalho para Applications. Arraste o aplicativo para esse diretório para instalar.
Os artefatos não recebem assinatura digital nem notarização por esses targets.

Use `make help` para consultar os comandos disponíveis.

### Linux

Em Debian/Ubuntu, instale as bibliotecas nativas antes do build:

```sh
sudo apt-get install build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev unixodbc-dev
wails dev -tags 'odbc,webkit2_41'
wails build -tags 'odbc,webkit2_41'
```

Instale e registre os drivers ODBC oficiais do Ignite correspondentes aos seus clusters.
O gerenciador unixODBC não inclui esses drivers.
O build sem `odbc` permite desenvolver os outros bancos e informa a ausência do transporte Ignite.

### Windows

Instale Go, Node.js e WebView2. O build Windows deste projeto dispensa CGO e compilador C.
Instale os drivers ODBC oficiais do Ignite, com a mesma arquitetura do aplicativo.

```powershell
wails dev
wails build
```

O adaptador ODBC usa o gerenciador nativo do Windows.

### macOS

Instale Xcode Command Line Tools. Para Ignite via JDBC, instale Java 17 ou superior e Maven.

```sh
mvn -f ignite-bridge/pom.xml package
export GO_DB_VIEWER_IGNITE_BRIDGE="$PWD/ignite-bridge/target/ignite-bridge.jar"
wails dev
wails build
cp ignite-bridge/target/*.jar build/bin/go-db-viewer.app/Contents/Resources/
```

Na distribuição, mantenha `ignite-bridge.jar`, `ignite2-jdbc.jar` e `ignite3-jdbc.jar` no mesmo diretório.
O aplicativo procura o processo auxiliar ao lado do executável ou em `Contents/Resources`.
Os drivers JDBC usam classloaders separados para evitar conflitos entre Ignite 2 e 3.

Quando ODBC não estiver disponível, informe uma URL JDBC no formulário:

```text
Ignite 2: jdbc:ignite:thin://localhost:10800/PUBLIC?user=usuario&password=senha
Ignite 3: jdbc:ignite:thin://localhost:10800?username=usuario&password=senha
```

O transporte é escolhido pela string antes da conexão. Erros não provocam troca automática de transporte.
O processo Java recebe credenciais por stdin. Ele não recebe credenciais nos argumentos nem registra mensagens JDBC.
Se o processo encerrar após timeout, reconecte antes de executar outro comando.

Para usar ODBC no macOS, instale unixODBC e um driver Ignite compatível. Compile com `-tags odbc`.
Essa combinação precisa de validação própria; o transporte JDBC é o padrão do build macOS.

## Uso

1. Clique em **Nova conexão** ou pressione `Alt+N`.
2. Escolha o banco e informe a string completa.
3. Use **Testar conexão** para testar uma sessão temporária.
4. Use **Salvar no cofre** para manter a conexão ou **Conectar sem salvar** para uma sessão temporária.
5. Expanda os objetos e clique em uma tabela ou coleção para visualizar os dados.
6. Abra uma aba de consulta e separe os comandos por linhas vazias.
7. Posicione o cursor no bloco desejado e execute com `Ctrl+Enter`. No macOS, também pode usar `Cmd+Enter`.

O atalho funciona somente quando o editor tem foco. Em uma linha vazia, ele não executa nada.
Linhas contendo somente espaços ou tabs também separam blocos. O ponto e vírgula não separa blocos no editor.
Linhas dentro de literais ou comentários SQL preservam o bloco, para não executar seu conteúdo como outro comando.
A seleção de texto não altera essa regra: o aplicativo usa o bloco do cursor principal.
O botão **Executar** usa o mesmo bloco e preserva o foco do editor durante o clique.
Blocos contendo somente comentários SQL não são enviados ao banco.

O backend identifica automaticamente consultas, DML e DDL antes de executar.
Ele resolve o comando principal de `WITH` e reconhece `RETURNING` do PostgreSQL e `OUTPUT` sem `INTO` do SQL Server.
Comandos sem suporte à classificação automática são rejeitados antes da execução.
O aplicativo nunca tenta outro caminho de execução após erro.

### Autocomplete de tabelas

No editor SQL, escreva `SELECT * FROM ` e pressione `Ctrl+Espaço`.
O autocomplete também funciona após `JOIN` e com prefixos como `public.cli`.

As sugestões mostram tabelas e views acessíveis no banco da conexão da aba, com o schema correspondente.
Use as setas para selecionar, Enter ou Tab para inserir e Esc para fechar.
O editor insere o nome qualificado com os delimitadores corretos do banco.
As sugestões não abrem dentro de strings ou comentários, nem em abas desconectadas.

Para sugerir colunas de uma tabela, escreva o alias seguido de ponto:

```sql
SELECT * FROM "public"."clientes" C
WHERE C.
```

Após `C.`, a lista abre automaticamente. Use setas, Enter ou Tab para inserir a coluna.
Em `WHERE` e `ON`, pressione `Ctrl+Espaço` para listar colunas sem informar um alias.
Se duas tabelas possuem uma coluna com o mesmo nome, a sugestão inclui o alias.
Aliases com `AS`, identificadores delimitados e tabelas de `JOIN` também são aceitos.
A resolução considera o bloco atual e o escopo da subquery. CTEs e tabelas derivadas não geram metadados físicos.

Com `Ctrl+clique` no nome da tabela ou no alias, abra a aba **Propriedades**.
No macOS, `Cmd+clique` também funciona. O qualificador em `C.nome` permite a mesma navegação.
A aba mostra colunas, tipos, nullable, defaults, chave primária, chaves estrangeiras e índices disponíveis.
Use **Atualizar propriedades** para recarregar os metadados. A aba não lê os registros da tabela.
Se a seção estiver vazia, a interface informa que nenhum item foi encontrado.
Se o driver não suportar a seção ou ocorrer uma falha, a interface informa essa condição.
No Ignite ODBC, as chaves estrangeiras ficam marcadas como não suportadas.
Campos adicionais dependem do catálogo do banco ou dos metadados JDBC.

O catálogo e os metadados ficam em cache por sessão durante 60 segundos.
Desconectar, atualizar os objetos ou concluir uma execução invalida esse cache.
Abas de propriedades não são restauradas ao reabrir o aplicativo. Os scripts nomeados continuam disponíveis na biblioteca local.
O autocomplete consulta somente metadados e não executa o script.
O editor MongoDB mantém os comandos em Extended JSON, sem autocomplete de tabelas SQL.

### Scripts nomeados e biblioteca local

Ao clicar em uma conexão salva, o aplicativo mostra **Novo script** e a lista de scripts dessa conexão.
Essa ação não conecta ao banco. Você pode criar, abrir e editar scripts offline.
Se a conexão já estiver ativa, o script reutiliza a sessão. Caso contrário, use **Conectar** na aba.

Use **Salvar** ou `Ctrl+S` para gravar o script. No macOS, também pode usar `Cmd+S`.
No primeiro salvamento, informe o nome. O nome deve ter entre 1 e 120 caracteres.
A mesma conexão não pode ter nomes repetidos, sem diferenciar maiúsculas e minúsculas.
Use **Renomear** para alterar o nome sem gravar alterações pendentes do conteúdo.

O marcador na aba e o status do editor indicam alterações não salvas.
O texto só é gravado ao usar **Salvar**. Alterações feitas durante uma gravação permanecem pendentes para o próximo salvamento.
Se ocorrer uma falha, o editor mantém o texto e apresenta o erro.

Fechar uma aba mantém o script na biblioteca. Ao fechar uma aba alterada, escolha **Salvar**, **Descartar alterações** ou **Cancelar**.
Ao sair do aplicativo, a mesma verificação ocorre para cada aba alterada.
Na próxima abertura, o aplicativo restaura as abas com a última versão salva, sem conectar automaticamente.
Alterações descartadas não são recuperadas.

Use **Abrir script** para acessar a biblioteca. A lista permite buscar pelo nome e filtrar por conexão.
Os scripts aparecem pela última alteração salva, da mais recente para a mais antiga.
Abrir um script já aberto seleciona sua aba existente.
Para remover um script da biblioteca, use **Excluir** e confirme a exclusão.

Scripts de conexões temporárias podem ser salvos em **Sem conexão salva**.
Remover uma conexão preserva seus scripts. Eles também aparecem nessa lista e podem ser vinculados a outra conexão do mesmo tipo.
O vínculo alterado passa a fazer parte do script no próximo **Salvar**.

A biblioteca e o estado das abas ficam em `workspace.json`, no diretório de configuração `go-db-viewer` do usuário.
No Linux, o caminho padrão é `~/.config/go-db-viewer/workspace.json`, respeitando `XDG_CONFIG_HOME`.
A posição do cursor, a seleção e as abas abertas são salvas automaticamente, separadas do conteúdo dos scripts.
A barra inferior informa o estado dessa gravação. O editor informa o estado do script.
Abas de propriedades, visualizações de dados e resultados não entram na biblioteca.

Na primeira abertura após a atualização, o aplicativo migra o workspace anterior para a versão 2.
Todos os scripts anteriores são importados. Nomes repetidos recebem sufixos numéricos.
Uma cópia do arquivo original recebe o sufixo `.v1-backup-<id>`.
A gravação permanece atômica. Arquivos inválidos são preservados com o sufixo `.invalid-<id>`.
As credenciais continuam no cofre do sistema. Resultados, erros e identificadores de sessão não entram no workspace.

### Organização dos comandos

Por exemplo, os três comandos abaixo ficam em blocos independentes:

```sql
CREATE TABLE exemplo (id INTEGER PRIMARY KEY);

INSERT INTO exemplo (id)
VALUES (1), (2), (3);

SELECT id
FROM exemplo
ORDER BY id;
```

Use um comando por bloco. Vários comandos separados por `;` no mesmo bloco são rejeitados antes da execução.
A validação também rejeita blocos procedurais e transações explícitas.
Não há gerenciamento de transações entre abas. As operações usam autocommit.
Para Oracle, o adaptador remove o ponto e vírgula final antes de enviar o comando ao driver.

O limite inicial é de 200 linhas/itens por consulta ou página de visualização.
O aplicativo permite configurar valores entre 1 e 100.000.
O limite de tamanho permanece em 8 MiB. Células e documentos têm limite de 64 KiB.
O aplicativo informa quando o retorno é interrompido pela quantidade ou pelo tamanho.
Se o limite de tamanho interromper uma página, use projeção ou filtros no editor para continuar sem saltar registros.
Quando não houver chave primária disponível, a interface informa que a ordem pode variar.
O separador dos painéis aceita arraste e setas do teclado.

### Configurações, temas e máximo de resultados

Use a aba **Configurações**, no topo do workspace, para alterar o tema e o máximo padrão de linhas/itens.
As alterações válidas são salvas automaticamente. A tela informa falhas e permite tentar novamente.
As configurações ficam em `settings.json`, no mesmo diretório de configuração da biblioteca de scripts.
Se o arquivo estiver inválido, o aplicativo preserva uma cópia `.invalid-<id>` e carrega os valores padrão.

Os temas disponíveis são **Claro**, **Escuro** e **Sistema**. O padrão inicial é **Sistema**.
Nesse modo, a interface acompanha as mudanças do sistema operacional.
A troca de tema preserva os textos, as seleções e os resultados das abas abertas.

O padrão inicial de resultados é **200**. A configuração aceita inteiros entre **1** e **100.000**.
O mesmo limite se aplica a consultas SQL, consultas MongoDB e páginas de visualização de tabelas ou coleções.
O rodapé mostra a quantidade retornada e o limite usado, por exemplo, `37/200` ou `200/200 máx`.
O indicador `máx` significa que a quantidade atingiu o limite; ele não informa quantos registros adicionais existem.
Em comandos com múltiplos resultados, o contador considera o total da execução e o limite é compartilhado.
Comandos de escrita sem retorno continuam mostrando as linhas afetadas ou a resposta do banco.

No campo **Máximo nesta aba**, abaixo dos resultados, digite `1000` e confirme com Enter ou ao sair do campo.
O campo também aceita colar `/1000`. A substituição vale somente para aquela aba, enquanto ela existir.
Use **Usar padrão** para remover a substituição e acompanhar o valor das configurações.
A biblioteca de scripts não salva essa substituição.

Alterar o limite não executa nem repete o comando. O novo valor vale para a próxima execução.
O contador do resultado atual conserva o limite usado anteriormente.
Na visualização de tabelas, a próxima carga com um limite diferente reinicia na primeira página.
Se o limite global mudar, as abas sem substituição usam o novo padrão na próxima execução.
Limites menores definidos no SQL ou no comando MongoDB continuam sendo respeitados.
Resultados grandes podem atingir o limite de tamanho antes da quantidade configurada.
Os catálogos de tabelas e os demais metadados mantêm seus limites independentes.

### MongoDB

Selecione um banco na árvore ou preencha **Banco** no editor. Use um objeto Extended JSON por bloco:

```json
{"find":"clientes","filter":{},"limit":100}
```

```json
{"insert":"clientes","documents":[{"_id":{"$numberLong":"9223372036854775807"},"nome":"Exemplo"}]}
```

Comandos disponíveis: `find`, `aggregate`, `listCollections`, `listIndexes`, `insert`, `update`, `delete`,
`findAndModify`, `count`, `distinct`, `create`, `drop`, `createIndexes`, `dropIndexes`, `collStats`, `dbStats` e `ping`.

O primeiro campo identifica o comando. O aplicativo não executa JavaScript ou expressões mongosh.
Se houver vários comandos no editor MongoDB, separe os objetos por linhas vazias e execute o bloco do cursor.
Campos de sessão, transação e `$db` não são aceitos. O banco vem da seleção do editor.
Os resultados usam Extended JSON canônico para preservar os tipos BSON.
O driver consome e fecha cursores. Retries de leitura e escrita ficam desativados.

## Credenciais e sessões

Os metadados ficam em `go-db-viewer/connections.json`, dentro de `os.UserConfigDir()`.
Esse arquivo contém somente identificador, nome, tipo e indicação de credencial salva.

Strings completas ficam no Secret Service do Linux, Windows Credential Manager ou macOS Keychain.
O cofre usa o serviço `go-db-viewer` e o identificador da conexão como chave.
Se o cofre estiver indisponível, use **Conectar sem salvar** com uma string preenchida.
Alterar uma conexão salva encerra sua sessão ativa. Reabra a conexão para usar os novos parâmetros.

O aplicativo mantém uma biblioteca local de scripts nomeados, mas não grava histórico de execuções ou resultados.
Mensagens públicas omitem mensagens externas que possam conter credenciais ou comandos.
Quando disponível, o erro inclui somente o código SQLSTATE validado.

Operações possuem timeout de 30 segundos. Falhas não repetem comandos.
Após timeout ou perda de conexão durante escrita, confira os dados antes de repetir manualmente.

## Arquitetura

- `internal/core`: contratos, sessões, limites, armazenamento e tratamento de erros.
- `internal/drivers`: adaptadores SQL, MongoDB, ODBC e processo JDBC.
- `internal/vault`: acesso ao cofre do sistema.
- `frontend`: interface Vue e editor CodeMirror.
- `ignite-bridge`: processo Java e pacotes JDBC isolados.

Os bindings Wails expõem perfis, conexão, navegação, catálogo de tabelas, resolução de referências, propriedades, descrição, visualização, execução, biblioteca de scripts e estado das abas.
Depois da conexão, o frontend usa identificadores de sessão.
As células SQL transportam texto, `null` e indicação de truncamento. Isso evita arredondamento no JavaScript.

## Testes

```sh
go test -race ./internal/...
go test -tags odbc ./internal/drivers
cd frontend
npm test
npm run build
npx playwright-core install chromium
npm run test:ui
```

Para usar um Chrome já instalado, configure `CHROME_PATH`.
Os testes de interface usam bindings simulados exclusivamente no navegador de teste.

Sem dependências externas, os testes principais também podem executar assim:

```sh
go test -race ./internal/core
go test -race internal/drivers/sql.go internal/drivers/statement.go internal/drivers/sql_test.go
go test -race internal/drivers/sql.go internal/drivers/statement.go internal/drivers/ignite.go internal/drivers/odbc_disabled.go internal/drivers/sql_test.go internal/drivers/ignite_test.go
```

Para testar bancos reais, configure as variáveis abaixo com conexões de teste:

```text
DBVIEWER_POSTGRESQL_DSN
DBVIEWER_ORACLE_DSN
DBVIEWER_SQLSERVER_DSN
DBVIEWER_MONGODB_DSN
DBVIEWER_IGNITE2_DSN
DBVIEWER_IGNITE3_DSN
DBVIEWER_MONGODB_DATABASE (opcional; padrão dbviewer_test)
```

```sh
go test -tags odbc -count=1 -run TestIntegration -v ./internal/drivers
```

A suíte cria e remove tabelas ou coleções temporárias. Use somente bancos de teste com permissões adequadas.
Ela verifica leitura, escrita, metadados, Unicode, NULL e precisão numérica.
A CI configura builds nos três sistemas e integração com PostgreSQL e MongoDB.
Oracle, SQL Server e ambas as versões Ignite exigem execução adicional com seus ambientes de teste.

## Estado da validação local

- Testes Go da camada comum, execução SQL e protocolo do processo auxiliar passaram com `-race`.
- Testes de estado das abas, seleção de blocos e build frontend passaram.
- A classificação automática SQL passou nos testes Go com `-race`, incluindo erros sem repetição de comandos.
- O build Linux compilou com as tags `production,odbc,webkit2_41`.
- O processo Java compilou com `javac --release 17` e Jackson local.
- A integração real permanece pendente: o sandbox bloqueou a porta PostgreSQL local e o acesso ao Docker.
- O empacotamento Maven depende do download dos plugins e drivers JDBC.
- Os testes visuais de seleção e a suíte completa da interface passaram após a liberação do ambiente.

O executável Windows amd64 e os pacotes `.deb` e `.rpm` foram gerados no Linux com os targets Make.
Os metadados e os arquivos dos pacotes Linux foram inspecionados.
O bundle macOS e a execução do aplicativo Windows exigem validação nos respectivos sistemas.
Esta implementação não declara compatibilidade validada com todos os servidores.

Edição direta nas células, exportação, túnel SSH e sincronização ficam fora desta versão.
