create table guild_vars (
    guild_id character varying primary key not null,
    script_command_prefix character varying not null default '.',
    quiet boolean not null default false,
    admin_role_id character varying,
    announcement character varying not null,
    delete_errors_after_seconds integer not null default 5,
    allow_unprivileged_unlinked_commands boolean not null default false
);

create table guild_links (
    guild_id character varying not null,
    link_name character varying(20) not null,
    owner_name character varying(20) not null,
    script_name character varying(20) not null,

    primary key (guild_id, link_name)
);

create index guild_links_guild_id_idx on guild_links (guild_id);
create index guild_links_guild_id_script_idx on guild_links (guild_id, owner_name, script_name);

create table user_channel_stats (
    user_id character varying not null,
    channel_id character varying not null,

    num_characters_sent bigint not null,
    num_messages_sent bigint not null,
    last_reset_time timestamp with time zone not null,

    primary key (user_id, channel_id)
);

create table execution_budgets (
    user_id character varying not null primary key,
    remaining_budget bigint not null,
    last_update_time timestamp with time zone not null
);
