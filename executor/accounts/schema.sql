create table accounts (
    name character varying primary key not null,
    password_hash character varying not null,
    time_limit_seconds integer not null,
    memory_limit integer not null,
    tmpfs_size integer not null
);
