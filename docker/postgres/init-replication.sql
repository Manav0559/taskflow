-- Runs once on the primary's first startup (mounted into
-- /docker-entrypoint-initdb.d/). Creates the role the replica uses to stream WAL.
CREATE ROLE replicator WITH REPLICATION LOGIN PASSWORD 'replicator';
