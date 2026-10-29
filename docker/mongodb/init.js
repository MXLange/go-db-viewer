const database = db.getSiblingDB('dbviewer');

database.createUser({
  user: 'dbviewer',
  pwd: 'LocalDev2026',
  roles: [{ role: 'readWrite', db: 'dbviewer' }],
});

// A coleção vazia mantém o banco disponível na navegação.
database.createCollection('connection_check');
