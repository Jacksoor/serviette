create table associations(
    user_id character varying primary key not null,
    name character varying not null,
    account_handle character varying not null,
    account_key character varying not null
);

create unique index associations_user_id_account_handle
    on associations(user_id, account_handle);

create unique index associations_user_id_name
    on associations(user_id, name);
