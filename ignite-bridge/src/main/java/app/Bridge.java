package app;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import java.io.*;
import java.net.*;
import java.nio.charset.StandardCharsets;
import java.nio.file.*;
import java.sql.*;
import java.util.*;

/** Um processo atende uma sessão. Falhas nunca repetem comandos. */
public final class Bridge {
    private static final ObjectMapper JSON = new ObjectMapper();
    private static final int MAX_ROWS = 1000, MAX_BYTES = 8 << 20, MAX_CELL = 65536;
    private Connection connection;
    private URLClassLoader loader;

    public static void main(String[] args) throws Exception {
        PrintStream protocol = System.out;
        System.setOut(System.err);
        Bridge bridge = new Bridge();
        try (BufferedReader input = new BufferedReader(new InputStreamReader(System.in, StandardCharsets.UTF_8))) {
            for (String line; (line = input.readLine()) != null;) {
                Object response;
                try {
                    if (line.length() > 2 << 20) throw new IOException("size");
                    response = bridge.handle(JSON.readTree(line));
                } catch (Exception error) {
                    // A resposta não contém mensagens JDBC, SQL ou credenciais.
                    response = Map.of("ok", false);
                }
                protocol.println(JSON.writeValueAsString(response));
                protocol.flush();
            }
        } finally {
            if (bridge.connection != null) bridge.connection.close();
            if (bridge.loader != null) bridge.loader.close();
        }
    }

    private Map<String, Object> handle(JsonNode request) throws Exception {
        String action = request.path("action").asText();
        if (action.equals("open")) {
            if (connection != null) throw new SQLException("already open");
            boolean ignite2 = request.path("kind").asText().equals("ignite2");
            Path jar = Paths.get(Bridge.class.getProtectionDomain().getCodeSource().getLocation().toURI());
            Path directory = Files.isDirectory(jar) ? jar : jar.getParent();
            Path driverJar = directory.resolve(ignite2 ? "ignite2-jdbc.jar" : "ignite3-jdbc.jar");
            loader = new URLClassLoader(new URL[]{driverJar.toUri().toURL()}, ClassLoader.getPlatformClassLoader());
            Thread.currentThread().setContextClassLoader(loader);
            String driverClass = ignite2 ? "org.apache.ignite.IgniteJdbcThinDriver" : "org.apache.ignite.jdbc.IgniteJdbcDriver";
            Driver driver = (Driver) Class.forName(driverClass, true, loader).getConstructor().newInstance();
            connection = driver.connect(request.path("dsn").asText(), new Properties());
            if (connection == null) throw new SQLException("connection");
            connection.setAutoCommit(true);
            return Map.of("ok", true);
        }
        if (connection == null) throw new SQLException("closed");
        if (action.equals("ping")) {
            try (Statement statement = statement(); ResultSet rows = statement.executeQuery("SELECT 1")) {
                if (!rows.next()) throw new SQLException("ping");
            }
            return Map.of("ok", true);
        }
        if (action.equals("list")) return Map.of("ok", true, "nodes", list(request.path("node")));
        if (action.equals("tables")) {
            List<Map<String, String>> tables = new ArrayList<>();
            try (ResultSet rows = connection.getMetaData().getTables(null, null, "%", new String[]{"TABLE", "VIEW"})) {
                while (rows.next()) {
                    if (tables.size() >= 10000) throw new SQLException("catalog size");
                    String schema = rows.getString("TABLE_SCHEM"), name = rows.getString("TABLE_NAME");
                    if (schema == null || name == null) continue;
                    tables.add(Map.of("schema", schema, "name", name, "insertText", quote(schema) + "." + quote(name)));
                }
            }
            return Map.of("ok", true, "tables", tables);
        }
        if (action.equals("resolve")) return Map.of("ok", true, "node", resolve(request.path("reference")));
        if (action.equals("properties")) return Map.of("ok", true, "properties", properties(request.path("node")));
        if (action.equals("describe")) {
            try (Statement statement = statement(); ResultSet rows = statement.executeQuery("SELECT * FROM " + table(request.path("node")) + " WHERE 1=0")) {
                return Map.of("ok", true, "columns", columns(rows.getMetaData()));
            }
        }
        if (action.equals("preview")) {
            JsonNode preview = request.path("preview"), node = preview.path("node");
            List<String> keys = new ArrayList<>();
            try (ResultSet rows = connection.getMetaData().getPrimaryKeys(null, node.path("schema").asText(), node.path("name").asText())) {
                TreeMap<Integer, String> sorted = new TreeMap<>();
                while (rows.next()) sorted.put(rows.getInt("KEY_SEQ"), quote(rows.getString("COLUMN_NAME")));
                keys.addAll(sorted.values());
            } catch (SQLFeatureNotSupportedException ignored) { }
            int offset = preview.path("page").asInt() * 100;
            String sql = "SELECT * FROM " + table(node) + (keys.isEmpty() ? "" : " ORDER BY " + String.join(",", keys)) + " LIMIT 101 OFFSET " + offset;
            try (Statement statement = statement()) {
                Map<String, Object> result = execute(statement, sql, 100);
                if (keys.isEmpty()) result.put("message", "Sem chave primária disponível. A ordem entre páginas pode variar.");
                return Map.of("ok", true, "result", result);
            }
        }
        if (action.equals("execute")) {
            try (Statement statement = statement()) {
                return Map.of("ok", true, "result", execute(statement, request.path("request").path("text").asText(), MAX_ROWS));
            }
        }
        throw new IOException("action");
    }

    private Map<String, String> resolve(JsonNode ref) throws SQLException {
        DatabaseMetaData md = connection.getMetaData();
        String name = ref.path("name").asText(), schema = ref.path("schema").asText();
        if (!ref.path("nameQuoted").asBoolean()) name = fold(md, name);
        if (!ref.path("schemaQuoted").asBoolean()) schema = fold(md, schema);
        if (schema.isEmpty()) {
            try { schema = connection.getSchema(); } catch (SQLFeatureNotSupportedException ignored) { }
        }
        Map<String, String> found = null;
        try (ResultSet rows = md.getTables(null, schema == null || schema.isEmpty() ? null : pattern(schema), pattern(name), new String[]{"TABLE", "VIEW"})) {
            while (rows.next()) {
                String actualSchema = rows.getString("TABLE_SCHEM"), actualName = rows.getString("TABLE_NAME");
                if (!name.equals(actualName) || actualSchema == null || (schema != null && !schema.isEmpty() && !schema.equals(actualSchema))) continue;
                if (found != null) throw new SQLException("ambiguous");
                found = Map.of("schema", actualSchema, "name", actualName, "kind", "table", "database", "Ignite");
            }
        }
        if (found == null) throw new SQLException("not found");
        return found;
    }
    private String fold(DatabaseMetaData md, String value) throws SQLException {
        return md.storesUpperCaseIdentifiers() ? value.toUpperCase(Locale.ROOT) : md.storesLowerCaseIdentifiers() ? value.toLowerCase(Locale.ROOT) : value;
    }
    @FunctionalInterface private interface MetadataReader { ResultSet get() throws SQLException; }
    private Map<String, Object> section(MetadataReader reader, String[] fields, String[] labels, String[] order) {
        try (ResultSet rows = reader.get()) {
            List<List<Object>> output = new ArrayList<>();
            List<List<String>> values = new ArrayList<>();
            int bytes = 0;
            while (rows.next()) {
                // Estatísticas de tabelas não são índices.
                if (Arrays.asList(fields).contains("NON_UNIQUE") && rows.getShort("TYPE") == DatabaseMetaData.tableIndexStatistic) continue;
                List<String> row = new ArrayList<>();
                for (String field : fields) {
                    String value = rows.getString(field);
                    if (value != null && (field.equals("UPDATE_RULE") || field.equals("DELETE_RULE"))) {
                        value = switch (Integer.parseInt(value)) {
                            case DatabaseMetaData.importedKeyCascade -> "CASCADE";
                            case DatabaseMetaData.importedKeySetNull -> "SET NULL";
                            case DatabaseMetaData.importedKeySetDefault -> "SET DEFAULT";
                            case DatabaseMetaData.importedKeyRestrict -> "RESTRICT";
                            case DatabaseMetaData.importedKeyNoAction -> "NO ACTION";
                            default -> "Indisponível";
                        };
                    }
                    bytes += value == null ? 64 : 64 + 6 * value.getBytes(StandardCharsets.UTF_8).length;
                    row.add(value);
                }
                if (values.size() >= 10000 || bytes > MAX_BYTES / 4) throw new SQLException("size");
                values.add(row);
            }
            values.sort((a,b) -> {
                for (String field : order) {
                    int index = Arrays.asList(fields).indexOf(field);
                    String left = a.get(index), right = b.get(index);
                    int comparison;
                    if (field.equals("KEY_SEQ") || field.equals("ORDINAL_POSITION")) comparison = Integer.compare(Integer.parseInt(left == null ? "0" : left), Integer.parseInt(right == null ? "0" : right));
                    else comparison = Objects.toString(left, "").compareTo(Objects.toString(right, ""));
                    if (comparison != 0) return comparison;
                }
                return 0;
            });
            for (List<String> row : values) {
                List<Object> cells = new ArrayList<>();
                for (String value : row) cells.add(Map.of("text", value == null ? "" : value, "null", value == null, "truncated", false));
                output.add(cells);
            }
            return Map.of("status", "available", "message", "", "columns", labels, "rows", output);
        } catch (SQLFeatureNotSupportedException error) {
            return Map.of("status", "unsupported", "message", "O driver JDBC não expõe estes metadados.", "columns", List.of(), "rows", List.of());
        } catch (SQLException | RuntimeException error) {
            return Map.of("status", "unavailable", "message", "Metadados indisponíveis. Verifique as permissões e a versão do driver.", "columns", List.of(), "rows", List.of());
        }
    }
    private Map<String, Object> properties(JsonNode node) throws SQLException {
        DatabaseMetaData md = connection.getMetaData();
        String schema = node.path("schema").asText(), name = node.path("name").asText();
        Map<String, Object> output = new HashMap<>();
        output.put("table", node);
        output.put("columns", section(() -> md.getColumns(null, pattern(schema), pattern(name), "%"),
            new String[]{"COLUMN_NAME","TYPE_NAME","IS_NULLABLE","COLUMN_DEF","COLUMN_SIZE","DECIMAL_DIGITS","ORDINAL_POSITION"},
            new String[]{"Coluna","Tipo","Aceita NULL","Default","Tamanho / precisão","Escala","Posição"}, new String[]{"ORDINAL_POSITION"}));
        output.put("primaryKey", section(() -> md.getPrimaryKeys(null, schema, name),
            new String[]{"PK_NAME","COLUMN_NAME","KEY_SEQ"}, new String[]{"Chave","Coluna","Posição"}, new String[]{"PK_NAME","KEY_SEQ"}));
        output.put("foreignKeys", section(() -> md.getImportedKeys(null, schema, name),
            new String[]{"FK_NAME","FKCOLUMN_NAME","PKTABLE_SCHEM","PKTABLE_NAME","PKCOLUMN_NAME","UPDATE_RULE","DELETE_RULE","KEY_SEQ"},
            new String[]{"Chave","Coluna","Schema destino","Tabela destino","Coluna destino","ON UPDATE","ON DELETE","Posição"}, new String[]{"FK_NAME","KEY_SEQ"}));
        output.put("indexes", section(() -> md.getIndexInfo(null, schema, name, false, true),
            new String[]{"INDEX_NAME","NON_UNIQUE","COLUMN_NAME","ORDINAL_POSITION","ASC_OR_DESC","FILTER_CONDITION"},
            new String[]{"Índice","Não único","Coluna","Posição","Ordem","Filtro"}, new String[]{"INDEX_NAME","ORDINAL_POSITION"}));
        return output;
    }

    private Statement statement() throws SQLException {
        Statement statement = connection.createStatement();
        try { statement.setQueryTimeout(30); } catch (SQLFeatureNotSupportedException ignored) { }
        statement.setFetchSize(100);
        return statement;
    }

    private List<Map<String, String>> list(JsonNode parent) throws SQLException {
        String kind = parent.path("kind").asText();
        List<Map<String, String>> nodes = new ArrayList<>();
        if (kind.isEmpty()) return List.of(Map.of("name", "Ignite", "kind", "database", "database", "Ignite", "schema", ""));
        if (kind.equals("database")) {
            try (ResultSet rows = connection.getMetaData().getSchemas()) {
                while (rows.next() && nodes.size() < 10000) nodes.add(Map.of("name", rows.getString("TABLE_SCHEM"), "kind", "schema", "database", "Ignite", "schema", rows.getString("TABLE_SCHEM")));
            }
        } else if (kind.equals("schema")) {
            try (ResultSet rows = connection.getMetaData().getTables(null, pattern(parent.path("name").asText()), "%", new String[]{"TABLE", "VIEW"})) {
                while (rows.next() && nodes.size() < 10000) nodes.add(Map.of("name", rows.getString("TABLE_NAME"), "kind", "table", "database", "Ignite", "schema", parent.path("name").asText()));
            }
        }
        return nodes;
    }

    private String pattern(String value) throws SQLException {
        String escape = connection.getMetaData().getSearchStringEscape();
        if (escape == null || escape.isEmpty()) return value;
        return value.replace(escape, escape + escape).replace("%", escape + "%").replace("_", escape + "_");
    }
    private String quote(String name) { return "\"" + name.replace("\"", "\"\"") + "\""; }
    private String table(JsonNode node) { return quote(node.path("schema").asText()) + "." + quote(node.path("name").asText()); }
    private List<Map<String, String>> columns(ResultSetMetaData metadata) throws SQLException {
        List<Map<String, String>> columns = new ArrayList<>();
        for (int i = 1; i <= metadata.getColumnCount(); i++) columns.add(Map.of("name", metadata.getColumnLabel(i), "type", metadata.getColumnTypeName(i), "insertText", quote(metadata.getColumnLabel(i))));
        return columns;
    }

    private Map<String, Object> execute(Statement statement, String sql, int limit) throws SQLException {
        Map<String, Object> result = new HashMap<>();
        List<Object> sets = new ArrayList<>();
        result.put("sets", sets); result.put("documents", List.of()); result.put("affected", "");
        result.put("message", ""); result.put("truncated", false); result.put("hasMore", false);
        int count = 0, bytes = 0;
        boolean hasRows = statement.execute(sql);
        for (;;) {
            if (hasRows) {
                try (ResultSet rows = statement.getResultSet()) {
                    List<Object> output = new ArrayList<>();
                    List<Map<String, String>> cols = columns(rows.getMetaData());
                    for (Map<String, String> column : cols) bytes += 64 + 6 * (column.get("name").getBytes(StandardCharsets.UTF_8).length + column.get("type").length());
                    if (bytes > MAX_BYTES) { result.put("truncated", true); result.put("message", "Metadados excedem o limite de tamanho."); return result; }
                    sets.add(Map.of("columns", cols, "rows", output));
                    while (rows.next()) {
                        if (count >= limit) { result.put("truncated", true); result.put("hasMore", true); return result; }
                        List<Object> cells = new ArrayList<>();
                        for (int i = 1; i <= cols.size(); i++) {
                            String value;
                            Object raw = rows.getObject(i);
                            if (raw instanceof byte[] binary) value = "base64:" + Base64.getEncoder().encodeToString(binary);
                            else if (raw instanceof java.math.BigDecimal decimal) value = decimal.toPlainString();
                            else value = rows.getString(i);
                            boolean truncated = false;
                            if (value != null) {
                                byte[] encoded = value.getBytes(StandardCharsets.UTF_8);
                                if (encoded.length > MAX_CELL) {
                                    int end = MAX_CELL;
                                    while ((encoded[end] & 0xc0) == 0x80) end--;
                                    value = new String(encoded, 0, end, StandardCharsets.UTF_8); truncated = true;
                                }
                                bytes += 64 + 6 * value.getBytes(StandardCharsets.UTF_8).length;
                            }
                            cells.add(Map.of("text", value == null ? "" : value, "null", value == null, "truncated", truncated));
                        }
                        if (bytes > MAX_BYTES) { result.put("truncated", true); result.put("message", "Limite de tamanho atingido. Use menos colunas ou registros."); return result; }
                        output.add(cells); count++;
                    }
                }
            } else {
                long affected;
                try { affected = statement.getLargeUpdateCount(); }
                catch (SQLFeatureNotSupportedException | UnsupportedOperationException unsupported) { affected = statement.getUpdateCount(); }
                if (affected == -1) break;
                result.put("affected", Long.toString(affected)); result.put("message", "Comando concluído.");
            }
            hasRows = statement.getMoreResults(Statement.CLOSE_CURRENT_RESULT);
        }
        return result;
    }
}
