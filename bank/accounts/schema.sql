create table accounts (
    id integer primary key autoincrement not null,
    key bytea not null,
    balance bigint not null default 0
);

create table asset_types (
    id integer primary key autoincrement not null,
    name character varying not null,
    price bigint not null default 0,
    duration_seconds bigint not null default 0
);

create unique index asset_types_name on asset_types(name);

create table assets (
    id integer primary key autoincrement not null,
    account_id integer not null,
    asset_type_id integer not null,
    name character varying not null,
    content blob not null,
    expiry_time_unix bigint not null,

    foreign key (account_id) references accounts(id)
        on update cascade
        on delete cascade,

    foreign key (asset_type_id) references asset_types(id)
        on update cascade
        on delete cascade
);

create unique index assets_account_id_type_name_idx
    on assets(account_id, asset_type_id, name);
