DROP DATABASE IF EXISTS test;
DROP ROLE IF EXISTS test;

CREATE DATABASE test;

\c test

CREATE ROLE test LOGIN SUPERUSER  PASSWORD '1234';

GRANT ALL PRIVILEGES ON DATABASE test TO test;

SET client_encoding TO 'UTF-8';

CREATE TABLE users(
	"id" SERIAL PRIMARY KEY,
	"name" CHARACTER VARYING(300),
  "login" CHARACTER VARYING(100) UNIQUE NOT NULL,
  "password" CHARACTER VARYING(300) NOT NULL,
	"type" CHARACTER VARYING(5) DEFAULT 'user',       --2 types of user can be record: user, admin
	"created_at" TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE posts(
	"id" SERIAL PRIMARY KEY,
	"header" CHARACTER VARYING(300),
  "text" TEXT,
	"created_at" TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);