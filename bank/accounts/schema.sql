create table accounts (
    handle bytea primary key not null,
    key bytea not null,
    balance bigint not null default 0
);

create table transfers (
    id integer primary key autoincrement not null,
    source_handle bytea,
    target_handle bytea not null,
    time_unix bigint not null,
    amount bigint not null,

    foreign key (source_handle) references accounts(handle)
        on update cascade
        on delete cascade,

    foreign key (target_handle) references accounts(handle)
        on update cascade
        on delete cascade
);

create index transfers_source_handle on transfers(source_handle);
create index transfers_target_handle on transfers(target_handle);
