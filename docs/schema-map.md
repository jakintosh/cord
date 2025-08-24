# Schema

# Schema Map

```sql
CREATE TABLE IF NOT EXISTS cidr (
    id                  INTEGER PRIMARY KEY,
    name                TEXT NOT NULL UNIQUE,
    cidr                TEXT NOT NULL UNIQUE,
    length              INTEGER NOT NULL,
    prefix              INTEGER NOT NULL,
    base                BLOB NOT NULL,
    last                BLOB NOT NULL,
    UNIQUE (base, prefix)
);

CREATE TABLE IF NOT EXISTS association (
    id                  INTEGER PRIMARY KEY,
    cidr1               INTEGER NOT NULL,
    cidr2               INTEGER NOT NULL,
    FOREIGN KEY (cidr1)
        REFERENCES cidr (id)
        ON UPDATE RESTRICT,
    FOREIGN KEY (cidr2)
        REFERENCES cidr (id)
        ON UPDATE RESTRICT
);

CREATE TABLE IF NOT EXISTS invite (
    id                  INTEGER PRIMARY KEY,
    public_key          TEXT NOT NULL UNIQUE,    -- temporary public key
    temp_cidr           TEXT NOT NULL UNIQUE,    -- CIDR on invite network
    final_cidr          INTEGER NOT NULL UNIQUE, -- CIDR for main network
    name                TEXT NOT NULL,           -- future peer name
    admin               INTEGER DEFAULT 0 NOT NULL,
    redeemed            INTEGER DEFAULT 0 NOT NULL,
    expiration          INTEGER NOT NULL,
    FOREIGN KEY (final_cidr)
        REFERENCES cidr (id)
);

CREATE TABLE IF NOT EXISTS peer (
    id                  INTEGER PRIMARY KEY,
    cidr                INTEGER NOT NULL,
    public_key          TEXT NOT NULL UNIQUE,
    admin               INTEGER DEFAULT 0 NOT NULL,
    disabled            INTEGER DEFAULT 0 NOT NULL,
    confirmed           INTEGER DEFAULT 0 NOT NULL,
    FOREIGN KEY (cidr)
        REFERENCES cidr (id)
);

CREATE TABLE IF NOT EXISTS endpoint (
    id                  INTEGER PRIMARY KEY,
    witness             INTEGER NOT NULL,
    peer                INTEGER NOT NULL,
    endpoint            TEXT NOT NULL,
    time                INTEGER NOT NULL,
    FOREIGN KEY (peer)
        REFERENCES peer (id),
    FOREIGN KEY (witness)
        REFERENCES peer (id)
);
```

## Client Schema

```sql
CREATE TABLE IF NOT EXISTS peer (
    id                  INTEGER PRIMARY KEY,
    public_key          TEXT NOT NULL UNIQUE,
    name                TEXT NOT NULL UNIQUE,
    ip                  INTEGER NOT NULL UNIQUE,
);

CREATE TABLE IF NOT EXISTS endpoint (
    id                  INTEGER PRIMARY KEY,
    peer_key            TEXT NOT NULL UNIQUE,
    peer_ip             BLOB NOT NULL UNIQUE,
    endpoint            TEXT NOT NULL,
    time                INTEGER NOT NULL
);
```
