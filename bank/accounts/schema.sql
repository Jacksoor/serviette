create table accounts (
    handle bytea primary key not null,
    key bytea not null,
    balance bigint not null default 0
);

create table aliases (
    name character varying primary key not null,
    account_handle bytea not null,

    foreign key (account_handle) references accounts(handle)
        on update cascade
        on delete cascade
);
