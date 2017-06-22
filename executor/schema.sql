create table accounts (
    name character varying primary key not null,
    password_hash character varying not null,
    time_limit_seconds integer not null default 5,
    memory_limit integer not null default 20971520,
    tmpfs_size integer not null default 20971520,
    allow_network_access boolean not null default false,
    allow_raw_output boolean not null default false,
    allowed_services character varying[] not null default array[]::character varying[]
);
