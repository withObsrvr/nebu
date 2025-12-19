# Simplified nebu deployment using pre-built Docker image
# No compilation needed - just pull and configure

FROM withobsrvr/nebu:latest

# Copy deployment scripts and migrations
COPY start.sh /start.sh
COPY 001_schema.sql /001_schema.sql

RUN chmod +x /start.sh

CMD ["/start.sh"]
