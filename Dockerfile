# Dockerfile
FROM nginx:alpine

# Add build argument for commit SHA
ARG COMMIT_SHA=unknown

# Copy all static files from the root directory
COPY . /usr/share/nginx/html

# Replace the placeholder in the main HTML file with the commit SHA
# This allows you to see which version of the code is deployed
RUN sed -i "s/__COMMIT_SHA__/${COMMIT_SHA}/g" /usr/share/nginx/html/index.html
