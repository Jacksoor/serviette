create table guild_vars (
    guild_id character varying primary key not null,
    script_command_prefix character varying not null,
    bank_command_prefix character varying not null,
    currency_name character varying not null,
    quiet boolean not null
);

create table channel_vars (
    channel_id character varying primary key not null,
    max_payout bigint not null,
    min_payout bigint not null,
    cooldown_seconds bigint not null
);

create table user_vars (
    user_id character varying primary key not null,
    account_handle bytea not null,
    last_payout_time_unix bigint not null
);
