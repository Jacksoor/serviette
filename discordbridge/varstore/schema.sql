create table guild_vars (
    guild_id character varying primary key not null,
    script_command_prefix character varying not null,
    meta_command_prefix character varying not null,
    quiet boolean not null,
    admin_role_id character varying not null
);

create table guild_aliases (
    guild_id character varying not null,
    alias_name character varying not null,
    owner_name character varying not null,
    script_name character varying not null,

    primary key (guild_id, alias_name)
);

create index guild_aliases_guild_id_idx on guild_aliases (guild_id);
