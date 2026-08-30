CREATE ROLE sqlconn LOGIN PASSWORD 'sqlconn';
CREATE ROLE sqlwriter LOGIN PASSWORD 'sqlwriter';

GRANT CONNECT ON DATABASE appx_test TO sqlconn, sqlwriter;
