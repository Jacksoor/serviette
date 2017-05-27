create table aliases (
    name character varying primary key not null,
    account_handle bytea not null,
    script_name string not null,
    expiry_time_unix bigint not null,

    foreign key (account_handle) references accounts(handle)
        on update cascade
        on delete cascade
);
