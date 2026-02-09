package detect

var dockerfileTemplates = map[string]string{
	"nestjs": `FROM node:22-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM node:22-alpine
WORKDIR /app
COPY --from=builder /app/dist ./dist
COPY --from=builder /app/node_modules ./node_modules
COPY --from=builder /app/package.json ./
EXPOSE 3000
CMD ["node", "dist/main.js"]
`,
	"nextjs": `FROM node:22-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM node:22-alpine
WORKDIR /app
COPY --from=builder /app/.next ./.next
COPY --from=builder /app/node_modules ./node_modules
COPY --from=builder /app/package.json ./
COPY --from=builder /app/public ./public
EXPOSE 3000
CMD ["npm", "start"]
`,
	"nuxt": `FROM node:22-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM node:22-alpine
WORKDIR /app
COPY --from=builder /app/.output ./.output
EXPOSE 3000
CMD ["node", ".output/server/index.mjs"]
`,
	"express": `FROM node:22-alpine
WORKDIR /app
COPY package*.json ./
RUN npm ci --omit=dev
COPY . .
EXPOSE 3000
CMD ["node", "index.js"]
`,
	"fastify": `FROM node:22-alpine
WORKDIR /app
COPY package*.json ./
RUN npm ci --omit=dev
COPY . .
EXPOSE 3000
CMD ["node", "index.js"]
`,
	"django": `FROM python:3.13-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY . .
EXPOSE 8000
CMD ["gunicorn", "--bind", "0.0.0.0:8000", "config.wsgi:application"]
`,
	"fastapi": `FROM python:3.13-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY . .
EXPOSE 8000
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"]
`,
	"flask": `FROM python:3.13-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY . .
EXPOSE 5000
CMD ["gunicorn", "--bind", "0.0.0.0:5000", "app:app"]
`,
	"go": `FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/app .

FROM alpine:3.21
COPY --from=builder /bin/app /bin/app
EXPOSE 8080
CMD ["/bin/app"]
`,
	"rust": `FROM rust:1.84-alpine AS builder
WORKDIR /app
COPY . .
RUN cargo build --release

FROM alpine:3.21
COPY --from=builder /app/target/release/app /bin/app
EXPOSE 8080
CMD ["/bin/app"]
`,
	"phoenix": `FROM elixir:1.18-alpine AS builder
WORKDIR /app
ENV MIX_ENV=prod
COPY mix.exs mix.lock ./
RUN mix deps.get --only prod && mix deps.compile
COPY . .
RUN mix release

FROM alpine:3.21
RUN apk add --no-cache libstdc++ openssl ncurses-libs
WORKDIR /app
COPY --from=builder /app/_build/prod/rel/app ./
EXPOSE 4000
CMD ["bin/app", "start"]
`,
	"laravel": `FROM php:8.4-fpm-alpine
RUN apk add --no-cache nginx && docker-php-ext-install pdo_mysql
WORKDIR /app
COPY . .
RUN composer install --no-dev --optimize-autoloader
EXPOSE 8000
CMD ["php", "artisan", "serve", "--host=0.0.0.0", "--port=8000"]
`,
	"rails": `FROM ruby:3.4-alpine AS builder
WORKDIR /app
COPY Gemfile Gemfile.lock ./
RUN bundle install --without development test
COPY . .
RUN bundle exec rake assets:precompile 2>/dev/null || true

FROM ruby:3.4-alpine
WORKDIR /app
COPY --from=builder /app .
EXPOSE 3000
CMD ["bundle", "exec", "rails", "server", "-b", "0.0.0.0"]
`,
	"spring-boot": `FROM eclipse-temurin:21-jdk-alpine AS builder
WORKDIR /app
COPY . .
RUN ./mvnw package -DskipTests

FROM eclipse-temurin:21-jre-alpine
COPY --from=builder /app/target/*.jar /app/app.jar
EXPOSE 8080
CMD ["java", "-jar", "/app/app.jar"]
`,
}

func GenerateDockerfile(fw *Framework) string {
	if t, ok := dockerfileTemplates[fw.Name]; ok {
		return t
	}
	return ""
}
