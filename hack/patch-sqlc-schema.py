#!/usr/bin/env python3
from pathlib import Path

path = Path("internal/db/sqlcgen/db.go")
text = path.read_text()
text = text.replace("return &Queries{db: db}", "return &Queries{db: wrapDBTX(db)}")
text = text.replace("\t\tdb: tx,", "\t\tdb: wrapDBTX(tx),")
path.write_text(text)
