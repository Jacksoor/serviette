create table accounts (
    name character varying primary key not null,
    password_hash character varying not null,
    time_limit_seconds integer not null,
    memory_limit integer not null,
    tmpfs_size integer not null,
    allow_network_access boolean not null
    allow_raw_output boolean not null,
    allowed_services character varying[] not null
);