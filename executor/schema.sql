create table accounts (
    name character varying(20) primary key not null,
    password_hash character varying not null,
    time_limit_seconds integer not null default 5,
    memory_limit integer not null default 20971520,
    tmpfs_size integer not null default 20971520,
    blkio_weight integer not null default 100,
    cpu_shares integer not null default 100,
    allow_network_access boolean not null default false,
    allowed_output_formats character varying[] not null default array['text', 'rich'],
    allowed_services character varying[] not null default array['Deputy', 'NetworkInfo'],
    max_messages_per_invocation integer not null default 10
);

create table scripts (
    owner_name character varying(20) not null,
    script_name character varying(20) not null,
    description text(200) not null default '',
    published boolean not null default false,
    votes integer not null default 0,

    primary key (owner_name, script_name),

    foreign key (owner_name) references accounts (name)
        on update restrict
        on delete restrict
);

create index scripts_owner_name_idx on scripts (owner_name);
create index scripts_ft_idx on scripts using gin (to_tsvector('english', script_name || ' ' || description));

create or replace function extract_hashtags(text) returns text[]
    as 'select array(select lower(m[1]) from regexp_matches($1, ''#(\S+)'', ''g'') m);'
    language sql
    immutable
    returns null on null input;

create index scripts_tags_idx on scripts using gin (extract_hashtags(description));

create table account_identifiers (
    account_name character varying(20) not null,
    visibility smallint not null,

    primary key (identifier, account_name),

    foreign key (account_name) references accounts (name)
        on update cascade
        on delete cascade
);

create index account_identifiers_identifier_idx on account_identifiers (identifier);
create index account_identifiers_account_name_idx on account_identifiers (account_name);
