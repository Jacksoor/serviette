create table accounts (
    handle bytea primary key not null,
    key bytea not null,
    balance bigint not null default 0
);

create table name_types (
    id integer primary key autoincrement not null,
    name character varying not null,
    price bigint not null default 0,
    duration_seconds bigint not null default 0
);

create unique index name_types_name on name_types(name);

create table names (
    id integer primary key autoincrement not null,
    owner_account_handle bytea not null,
    name_type_id integer not null,
    name character varying not null,
    content blob not null,
    expiry_time_unix bigint not null,

    foreign key (owner_account_handle) references accounts(handle)
        on update cascade
        on delete cascade,

    foreign key (name_type_id) references name_types(id)
        on update cascade
        on delete cascade
);

create unique index names_name_type_id_name_idx
    on names(name_type_id, name);
