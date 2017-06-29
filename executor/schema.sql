create table accounts (
    name character varying primary key not null,
    password_hash character varying not null,
    time_limit_seconds integer not null default 5,
    memory_limit integer not null default 20971520,
    tmpfs_size integer not null default 20971520,
    allow_network_access boolean not null default false,
    allowed_output_formats character varying[] not null default array['text']::character varying[],
    allowed_services character varying[] not null default array[]::character varying[]
);

create table scripts (
    owner_name character varying not null,
    script_name character varying not null,
    description text not null default '',
    published boolean not null default false,

    primary key (owner_name, script_name)
);

create index scripts_owner_name_idx on scripts (owner_name);
create index scripts_ft_idx on scripts using gin (to_tsvector('english', 'script_name' || ' ' || 'description'));
