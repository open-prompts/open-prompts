# Build stage
FROM node:24.12.0-alpine AS builder
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm install
COPY . .
RUN npm run build
# Pre-compress static assets
# Switch to a faster domestic mirror for apk
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories
RUN apk add gzip && \
    find build -type f -regex ".*\.\(html\|js\|css\|json\)$" -exec gzip -9 -k {} \;


# Run stage
FROM nginx:alpine
COPY --from=builder /app/build /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
