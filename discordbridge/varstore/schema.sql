create table guild_vars (
    guild_id character varying primary key not null,
    script_command_prefix character varying not null,
    quiet boolean not null,
    admin_role_id character varying not null,
    announcement character varying not null
);

create table guild_links (
    guild_id character varying not null,
    link_name character varying not null,
    owner_name character varying not null,
    script_name character varying not null,

    primary key (guild_id, link_name)
);

create index guild_links_guild_id_idx on guild_links (guild_id);
