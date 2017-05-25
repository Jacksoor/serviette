create table accounts (
    handle bytea primary key not null,
    key bytea not null,
    balance bigint not null default 0
);

create table deed_types (
    id integer primary key autoincrement not null,
    name character varying not null,
    price bigint not null default 0,
    duration_seconds bigint not null default 0
);

create unique index deed_types_name on deed_types(name);

insert into deed_types(name, price, duration_seconds) values('command', 500, 8640000);

create table deeds (
    id integer primary key autoincrement not null,
    owner_account_handle bytea not null,
    deed_type_id integer not null,
    name character varying not null,
    content blob not null,
    expiry_time_unix bigint not null,

    foreign key (owner_account_handle) references accounts(handle)
        on update cascade
        on delete cascade,

    foreign key (deed_type_id) references deed_types(id)
        on update cascade
        on delete cascade
);

create unique index deeds_deed_type_id_name_idx
    on deeds(deed_type_id, name);

create table aliases (
    name character varying primary key not null,
    account_handle bytea not null,

    foreign key (account_handle) references accounts(handle)
        on update cascade
        on delete cascade
);
