DROP TABLE IF EXISTS user;
CREATE TABLE user (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    age INT NOT NULL
);
insert into user values ('00000000000000000000000001', 'hanako', 20);
insert into user values ('00000000000000000000000002', 'taro', 30);