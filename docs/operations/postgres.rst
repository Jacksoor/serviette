Postgres
========

kobun4 uses Postgres for data storage. The executor and each bridge require their own database, and their schemas are available in ``schema.sql`` in each component's directory.

Each component should have its own Postgres user, to ensure isolation between processes.
