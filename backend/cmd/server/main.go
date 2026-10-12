package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	pb "open-prompts/backend/api/proto/v1"
	"open-prompts/backend/internal/data"
	"open-prompts/backend/internal/models"
	"open-prompts/backend/internal/repository"
	"open-prompts/backend/internal/service"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func writeError(w http.ResponseWriter, err error) {
	zap.S().Errorf("Error handling request: %v", err)
	st, ok := status.FromError(err)
	if !ok {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var code int
	switch st.Code() {
	case codes.OK:
		code = http.StatusOK
	case codes.InvalidArgument:
		code = http.StatusBadRequest
	case codes.NotFound:
		code = http.StatusNotFound
	case codes.AlreadyExists:
		code = http.StatusConflict
	case codes.PermissionDenied:
		code = http.StatusForbidden
	case codes.Unauthenticated:
		code = http.StatusUnauthorized
	case codes.Unimplemented:
		code = http.StatusNotImplemented
	case codes.Unavailable:
		code = http.StatusServiceUnavailable
	case codes.ResourceExhausted:
		code = http.StatusTooManyRequests
	case codes.FailedPrecondition:
		code = http.StatusConflict
	default:
		code = http.StatusInternalServerError
	}

	http.Error(w, st.Message(), code)
}

func main() {
	logger, _ := zap.NewProduction()
	defer func() { _ = logger.Sync() }()
	zap.ReplaceGlobals(logger)

	zap.S().Info("Starting application...")
	// Database connection
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/open_prompts?sslmode=disable"
	}
	pgConn, err := data.NewPostgresConnection(dsn)
	if err != nil {
		zap.S().Fatalf("failed to connect to database: %v", err)
	}

	// Repository and Service
	templateRepo := repository.NewTemplateRepository(pgConn.DB)
	promptRepo := repository.NewPromptRepository(pgConn.DB)
	templateVersionRepo := repository.NewTemplateVersionRepository(pgConn.DB)
	templateAliasRepo := repository.NewTemplateAliasRepository(pgConn.DB)
	notificationRepo := repository.NewNotificationRepository(pgConn.DB)

	// User repository is needed by the prompt service for notification
	// actor lookups; construct it before the prompt service.
	userRepo := repository.NewUserRepository(pgConn.DB)

	svc := service.NewPromptService(promptRepo, templateRepo, templateVersionRepo, templateAliasRepo, notificationRepo, userRepo)

	// Redis
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	redisClient, err := data.NewRedisClient(redisAddr, "", 0)
	if err != nil {
		zap.S().Fatalf("failed to connect to redis: %v", err)
	}

	// User Service
	apiKeyRepo := repository.NewAPIKeyRepository(pgConn.DB)

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "default-secret-key-change-me"
	}

	// Email Service
	smtpHost := os.Getenv("SMTP_HOST")
	smtpPort := os.Getenv("SMTP_PORT")
	smtpUser := os.Getenv("SMTP_USER")
	smtpPassword := os.Getenv("SMTP_PASSWORD")
	smtpFrom := os.Getenv("SMTP_FROM")
	if smtpFrom == "" {
		smtpFrom = "noreply@open-prompts.com"
	}
	emailSvc := service.NewEmailService(smtpHost, smtpPort, smtpUser, smtpPassword, smtpFrom)

	userSvc := service.NewUserService(userRepo, apiKeyRepo, redisClient, emailSvc, jwtSecret)

	// Admin Service
	adminRepo := repository.NewAdminRepository(pgConn.DB)
	sessionTTLMinutes := 30
	if v := os.Getenv("ADMIN_SESSION_TTL_MINUTES"); v != "" {
		if parsed, perr := strconv.Atoi(v); perr == nil && parsed > 0 {
			sessionTTLMinutes = parsed
		}
	}
	adminSvc := service.NewAdminService(userRepo, adminRepo, apiKeyRepo, redisClient, jwtSecret, time.Duration(sessionTTLMinutes)*time.Minute)

	// Notification Service (engagement notifications)
	notificationReadRetentionDays := 90
	if v := os.Getenv("NOTIFICATION_READ_RETENTION_DAYS"); v != "" {
		if parsed, perr := strconv.Atoi(v); perr == nil && parsed > 0 {
			notificationReadRetentionDays = parsed
		}
	}
	notificationRetentionDays := 180
	if v := os.Getenv("NOTIFICATION_RETENTION_DAYS"); v != "" {
		if parsed, perr := strconv.Atoi(v); perr == nil && parsed > 0 {
			notificationRetentionDays = parsed
		}
	}
	notificationSvc := service.NewNotificationService(notificationRepo, notificationReadRetentionDays, notificationRetentionDays)

	// Auth Interceptor
	authInterceptor := service.NewAuthInterceptor(jwtSecret, apiKeyRepo)
	authInterceptor.SetUserRepository(userRepo)
	authInterceptor.SetRedis(redisClient)

	// Admin bootstrap (no-op when ADMIN_BOOTSTRAP_EMAIL is empty)
	if err := service.BootstrapAdmin(context.Background(), userRepo, os.Getenv("ADMIN_BOOTSTRAP_EMAIL"), os.Getenv("ADMIN_BOOTSTRAP_PASSWORD")); err != nil {
		zap.S().Errorf("admin bootstrap failed: %v", err)
	}

	// gRPC Server
	go func() {
		lis, err := net.Listen("tcp", ":50051")
		if err != nil {
			zap.S().Fatalf("failed to listen: %v", err)
		}

		// Register Interceptor
		s := grpc.NewServer(
			grpc.UnaryInterceptor(authInterceptor.Unary()),
		)

		pb.RegisterPromptServiceServer(s, svc)
		pb.RegisterUserServiceServer(s, userSvc)
		pb.RegisterAdminServiceServer(s, adminSvc)
		pb.RegisterNotificationServiceServer(s, notificationSvc)
		zap.S().Infof("gRPC server listening at %v", lis.Addr())
		if err := s.Serve(lis); err != nil {
			zap.S().Fatalf("failed to serve: %v", err)
		}
	}()

	// Helper for JSON marshaling
	marshaler := protojson.MarshalOptions{
		EmitUnpopulated: true,
		UseProtoNames:   true,
	}
	unmarshaler := protojson.UnmarshalOptions{
		DiscardUnknown: true,
	}

	// HTTP Server for FVT/REST
	http.HandleFunc("/api/v1/categories", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			return
		}

		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		req := &pb.ListCategoriesRequest{}
		if v := r.URL.Query().Get("owner_id"); v != "" {
			req.OwnerId = v
		}
		if v := r.URL.Query().Get("language"); v != "" {
			req.Language = v
		}

		resp, err := svc.ListCategories(context.Background(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		b, _ := marshaler.Marshal(resp)
		_, _ = w.Write(b)
	})

	http.HandleFunc("/api/v1/tags", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			return
		}

		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		req := &pb.ListTagsRequest{}
		if v := r.URL.Query().Get("language"); v != "" {
			req.Language = v
		}

		resp, err := svc.ListTags(context.Background(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		b, _ := marshaler.Marshal(resp)
		_, _ = w.Write(b)
	})

	http.HandleFunc("/api/v1/templates", func(w http.ResponseWriter, r *http.Request) {
		// Enable CORS
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			return
		}

		switch r.Method {
		case http.MethodGet:
			// Optional Auth for Mixed View
			ctx := context.Background()
			if authHeader := r.Header.Get("Authorization"); authHeader != "" {
				tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
				if userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr); err == nil {
					ctx = service.ContextWithUserID(ctx, userID)
				} else {
					// Optional: fail if token matches format but is invalid?
					// For now, let's just log and continue as anonymous (safe fallback)
					// Or strictly fail:
					http.Error(w, "Invalid token", http.StatusUnauthorized)
					return
				}
			}

			req := &pb.ListTemplatesRequest{}
			q := r.URL.Query()
			if v := q.Get("page_size"); v != "" {
				if i, err := strconv.Atoi(v); err == nil {
					req.PageSize = int32(i)
				}
			}
			req.PageToken = q.Get("page_token")
			req.OwnerId = q.Get("owner_id")
			req.Category = q.Get("category")
			req.Language = q.Get("language")
			if v := q.Get("visibility"); v != "" {
				switch v {
				case "VISIBILITY_PUBLIC":
					req.Visibility = pb.Visibility_VISIBILITY_PUBLIC
				case "VISIBILITY_PRIVATE":
					req.Visibility = pb.Visibility_VISIBILITY_PRIVATE
				}
			}
			req.Tags = q["tags"] // Supports ?tags=a&tags=b
			if len(req.Tags) == 0 {
				req.Tags = q["tags[]"] // Supports ?tags[]=a&tags[]=b
			}
			if v := q.Get("my_likes"); v == "true" {
				req.MyLikes = true
			}
			if v := q.Get("my_favorites"); v == "true" {
				req.MyFavorites = true
			}

			resp, err := svc.ListTemplates(ctx, req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)

		case http.MethodPost:
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Authorization header required", http.StatusUnauthorized)
				return
			}
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr)
			if err != nil {
				http.Error(w, "Invalid token", http.StatusUnauthorized)
				return
			}
			ctx := service.ContextWithUserID(context.Background(), userID)

			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read body", http.StatusBadRequest)
				return
			}
			var req pb.CreateTemplateRequest // Create Request
			if err := unmarshaler.Unmarshal(body, &req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			// Map string visibility values to proto enum (protojson only understands
			// numeric or full enum names like "VISIBILITY_PUBLIC", not "public")
			if req.Visibility == pb.Visibility_VISIBILITY_UNSPECIFIED {
				var rawJSON map[string]interface{}
				if err := json.Unmarshal(body, &rawJSON); err == nil {
					if vis, ok := rawJSON["visibility"].(string); ok {
						switch vis {
						case "public":
							req.Visibility = pb.Visibility_VISIBILITY_PUBLIC
						case "private":
							req.Visibility = pb.Visibility_VISIBILITY_PRIVATE
						}
					}
				}
			}
			// If client did not provide language, try to derive from Accept-Language header
			if req.Language == "" {
				if al := r.Header.Get("Accept-Language"); al != "" {
					// Take first language tag, then primary subtag
					first := strings.Split(al, ",")[0]
					lang := strings.Split(strings.TrimSpace(first), "-")[0]
					req.Language = strings.ToLower(lang)
				}
			}
			resp, err := svc.CreateTemplate(ctx, &req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("/api/v1/templates/", func(w http.ResponseWriter, r *http.Request) {
		// Enable CORS
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			return
		}

		id := strings.TrimPrefix(r.URL.Path, "/api/v1/templates/")
		if id == "" {
			http.Error(w, "ID required", http.StatusBadRequest)
			return
		}

		parts := strings.Split(id, "/")
		zap.S().Infof("DEBUG HTTP ROUTER id='%s' len(parts)=%d", id, len(parts))
		if len(parts) >= 2 && parts[1] == "aliases" {
			templateID := parts[0]
			if len(parts) == 2 {
				// /api/v1/templates/{template_id}/aliases
				switch r.Method {
				case http.MethodGet:
					req := &pb.ListAliasesRequest{TemplateId: templateID}
					resp, err := svc.ListAliases(context.Background(), req)
					if err != nil {
						writeError(w, err)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					b, _ := marshaler.Marshal(resp)
					_, _ = w.Write(b)
					return
				case http.MethodPost:
					authHeader := r.Header.Get("Authorization")
					if authHeader == "" {
						http.Error(w, "Authorization header required", http.StatusUnauthorized)
						return
					}
					tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
					userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr)
					if err != nil {
						http.Error(w, "Invalid token", http.StatusUnauthorized)
						return
					}
					ctx := service.ContextWithUserID(context.Background(), userID)

					body, err := io.ReadAll(r.Body)
					if err != nil {
						http.Error(w, "Failed to read body", http.StatusBadRequest)
						return
					}
					var req pb.CreateAliasRequest
					if err := unmarshaler.Unmarshal(body, &req); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					req.TemplateId = templateID
					resp, err := svc.CreateAlias(ctx, &req)
					if err != nil {
						writeError(w, err)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					b, _ := marshaler.Marshal(resp)
					_, _ = w.Write(b)
					return
				default:
					http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
					return
				}
			} else if len(parts) == 3 {
				// /api/v1/templates/{template_id}/aliases/{alias_name}
				aliasName := parts[2]
				switch r.Method {
				case http.MethodGet:
					req := &pb.GetPromptByAliasRequest{
						TemplateId: templateID,
						AliasName:  aliasName,
					}
					resp, err := svc.GetPromptByAlias(context.Background(), req)
					if err != nil {
						writeError(w, err)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					b, _ := marshaler.Marshal(resp)
					_, _ = w.Write(b)
					return
				case http.MethodPut:
					authHeader := r.Header.Get("Authorization")
					if authHeader == "" {
						http.Error(w, "Authorization header required", http.StatusUnauthorized)
						return
					}
					tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
					userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr)
					if err != nil {
						http.Error(w, "Invalid token", http.StatusUnauthorized)
						return
					}
					ctx := service.ContextWithUserID(context.Background(), userID)

					body, err := io.ReadAll(r.Body)
					if err != nil {
						http.Error(w, "Failed to read body", http.StatusBadRequest)
						return
					}
					var req pb.UpdateAliasRequest
					if err := unmarshaler.Unmarshal(body, &req); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					req.TemplateId = templateID
					req.AliasName = aliasName
					resp, err := svc.UpdateAlias(ctx, &req)
					if err != nil {
						writeError(w, err)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					b, _ := marshaler.Marshal(resp)
					_, _ = w.Write(b)
					return
				case http.MethodDelete:
					authHeader := r.Header.Get("Authorization")
					if authHeader == "" {
						http.Error(w, "Authorization header required", http.StatusUnauthorized)
						return
					}
					tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
					userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr)
					if err != nil {
						http.Error(w, "Invalid token", http.StatusUnauthorized)
						return
					}
					ctx := service.ContextWithUserID(context.Background(), userID)

					req := &pb.DeleteAliasRequest{
						TemplateId: templateID,
						AliasName:  aliasName,
					}
					resp, err := svc.DeleteAlias(ctx, req)
					if err != nil {
						writeError(w, err)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					b, _ := marshaler.Marshal(resp)
					_, _ = w.Write(b)
					return
				default:
					http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
					return
				}
			}
		}

		if strings.HasSuffix(id, "/fork") {
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Authorization header required", http.StatusUnauthorized)
				return
			}
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr)
			if err != nil {
				http.Error(w, "Invalid token", http.StatusUnauthorized)
				return
			}
			ctx := service.ContextWithUserID(context.Background(), userID)

			templateID := strings.TrimSuffix(id, "/fork")
			resp, err := svc.ForkTemplate(ctx, templateID)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
			return
		}

		if strings.HasSuffix(id, "/versions") {
			if r.Method != http.MethodGet {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			templateID := strings.TrimSuffix(id, "/versions")
			req := &pb.ListTemplateVersionsRequest{TemplateId: templateID}

			q := r.URL.Query()
			if v := q.Get("page_size"); v != "" {
				if i, err := strconv.Atoi(v); err == nil {
					req.PageSize = int32(i)
				}
			}
			req.PageToken = q.Get("page_token")

			resp, err := svc.ListTemplateVersions(context.Background(), req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
			return
		}

		if strings.HasSuffix(id, "/like") {
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Authorization header required", http.StatusUnauthorized)
				return
			}
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr)
			if err != nil {
				http.Error(w, "Invalid token", http.StatusUnauthorized)
				return
			}
			ctx := service.ContextWithUserID(context.Background(), userID)

			templateID := strings.TrimSuffix(id, "/like")
			req := &pb.ToggleLikeRequest{TemplateId: templateID}
			resp, err := svc.ToggleLikeTemplate(ctx, req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
			return
		}

		if strings.HasSuffix(id, "/favorite") {
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Authorization header required", http.StatusUnauthorized)
				return
			}
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr)
			if err != nil {
				http.Error(w, "Invalid token", http.StatusUnauthorized)
				return
			}
			ctx := service.ContextWithUserID(context.Background(), userID)

			templateID := strings.TrimSuffix(id, "/favorite")
			req := &pb.ToggleFavoriteRequest{TemplateId: templateID}
			resp, err := svc.ToggleFavoriteTemplate(ctx, req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
			return
		}

		switch r.Method {
		case http.MethodGet:
			ctx := context.Background()
			if authHeader := r.Header.Get("Authorization"); authHeader != "" {
				tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
				if userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr); err == nil {
					ctx = service.ContextWithUserID(ctx, userID)
				}
			}
			req := &pb.GetTemplateRequest{Id: id}
			resp, err := svc.GetTemplate(ctx, req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)

		case http.MethodPut:
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Authorization header required", http.StatusUnauthorized)
				return
			}
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr)
			if err != nil {
				http.Error(w, "Invalid token", http.StatusUnauthorized)
				return
			}
			ctx := service.ContextWithUserID(context.Background(), userID)

			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read body", http.StatusBadRequest)
				return
			}
			var req pb.UpdateTemplateRequest
			if err := unmarshaler.Unmarshal(body, &req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			// Map string visibility values to proto enum (protojson only understands
			// numeric or full enum names like "VISIBILITY_PUBLIC", not "public")
			if req.Visibility == pb.Visibility_VISIBILITY_UNSPECIFIED {
				var rawJSON map[string]interface{}
				if err := json.Unmarshal(body, &rawJSON); err == nil {
					if vis, ok := rawJSON["visibility"].(string); ok {
						switch vis {
						case "public":
							req.Visibility = pb.Visibility_VISIBILITY_PUBLIC
						case "private":
							req.Visibility = pb.Visibility_VISIBILITY_PRIVATE
						}
					}
				}
			}
			// If client did not provide language on update, derive from Accept-Language header
			if req.Language == "" {
				if al := r.Header.Get("Accept-Language"); al != "" {
					first := strings.Split(al, ",")[0]
					lang := strings.Split(strings.TrimSpace(first), "-")[0]
					req.Language = strings.ToLower(lang)
				}
			}
			req.TemplateId = id
			resp, err := svc.UpdateTemplate(ctx, &req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)

		case http.MethodDelete:
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Authorization header required", http.StatusUnauthorized)
				return
			}
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr)
			if err != nil {
				http.Error(w, "Invalid token", http.StatusUnauthorized)
				return
			}
			ctx := service.ContextWithUserID(context.Background(), userID)

			ownerID := r.URL.Query().Get("owner_id")
			req := &pb.DeleteTemplateRequest{Id: id, OwnerId: ownerID}
			resp, err := svc.DeleteTemplate(ctx, req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// User Handlers
	http.HandleFunc("/api/v1/verification-code", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusBadRequest)
			return
		}
		var req pb.SendVerificationCodeRequest
		if err := unmarshaler.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp, err := userSvc.SendVerificationCode(context.Background(), &req)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		b, _ := marshaler.Marshal(resp)
		_, _ = w.Write(b)
	})

	http.HandleFunc("/api/v1/register", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusBadRequest)
			return
		}
		var req pb.RegisterRequest
		if err := unmarshaler.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp, err := userSvc.Register(context.Background(), &req)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		b, _ := marshaler.Marshal(resp)
		_, _ = w.Write(b)
	})

	http.HandleFunc("/api/v1/login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusBadRequest)
			return
		}
		var req pb.LoginRequest
		if err := unmarshaler.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp, err := userSvc.Login(context.Background(), &req)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		b, _ := marshaler.Marshal(resp)
		_, _ = w.Write(b)
	})

	http.HandleFunc("/api/v1/profile", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr)
		if err != nil {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}
		ctx := service.ContextWithUserID(context.Background(), userID)

		switch r.Method {
		case http.MethodPut:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read body", http.StatusBadRequest)
				return
			}
			var req pb.UpdateProfileRequest
			if err := unmarshaler.Unmarshal(body, &req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			req.Id = userID

			resp, err := userSvc.UpdateProfile(ctx, &req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)

		case http.MethodGet:
			req := &pb.GetProfileRequest{}
			req.Id = userID

			resp, err := userSvc.GetProfile(ctx, req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// API Key Handlers
	http.HandleFunc("/api/v1/api-keys", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr)
		if err != nil {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}
		ctx := service.ContextWithUserID(context.Background(), userID)

		switch r.Method {
		case http.MethodGet:
			q := r.URL.Query()
			req := &pb.ListAPIKeysRequest{}
			if v := q.Get("page_size"); v != "" {
				if i, err := strconv.Atoi(v); err == nil {
					req.PageSize = int32(i)
				}
			}
			req.PageToken = q.Get("page_token")
			resp, err := userSvc.ListAPIKeys(ctx, req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)

		case http.MethodPost:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read body", http.StatusBadRequest)
				return
			}
			var req pb.CreateAPIKeyRequest
			if err := unmarshaler.Unmarshal(body, &req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			resp, err := userSvc.CreateAPIKey(ctx, &req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("/api/v1/api-keys/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr)
		if err != nil {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}
		ctx := service.ContextWithUserID(context.Background(), userID)

		id := strings.TrimPrefix(r.URL.Path, "/api/v1/api-keys/")
		if id == "" {
			http.Error(w, "ID required", http.StatusBadRequest)
			return
		}

		if r.Method == http.MethodDelete {
			req := &pb.DeleteAPIKeyRequest{Id: id}
			resp, err := userSvc.DeleteAPIKey(ctx, req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Notification Handlers (user-facing; recipient always from the token)
	http.HandleFunc("/api/v1/notifications", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr)
		if err != nil {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}
		ctx := service.ContextWithUserID(context.Background(), userID)

		q := r.URL.Query()
		req := &pb.ListNotificationsRequest{Filter: q.Get("filter")}
		if v := q.Get("page_size"); v != "" {
			if i, cerr := strconv.Atoi(v); cerr == nil {
				req.PageSize = int32(i)
			}
		}
		req.PageToken = q.Get("page_token")

		resp, err := notificationSvc.ListNotifications(ctx, req)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		b, _ := marshaler.Marshal(resp)
		_, _ = w.Write(b)
	})

	http.HandleFunc("/api/v1/notifications/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr)
		if err != nil {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}
		ctx := service.ContextWithUserID(context.Background(), userID)

		rest := strings.TrimPrefix(r.URL.Path, "/api/v1/notifications/")
		parts := strings.Split(rest, "/")

		// /api/v1/notifications/unread-count (GET)
		if len(parts) == 1 && parts[0] == "unread-count" {
			if r.Method != http.MethodGet {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			resp, err := notificationSvc.GetUnreadNotificationCount(ctx, &pb.GetUnreadNotificationCountRequest{})
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
			return
		}

		// /api/v1/notifications/read-all (POST)
		if len(parts) == 1 && parts[0] == "read-all" {
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			resp, err := notificationSvc.MarkAllNotificationsRead(ctx, &pb.MarkAllNotificationsReadRequest{})
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
			return
		}

		// /api/v1/notifications/{id}/read (POST)
		if len(parts) == 2 && parts[1] == "read" {
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			resp, err := notificationSvc.MarkNotificationRead(ctx, &pb.MarkNotificationReadRequest{Id: parts[0]})
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
			return
		}

		// /api/v1/notifications/{id} (DELETE)
		if len(parts) == 1 && parts[0] != "" {
			if r.Method != http.MethodDelete {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			resp, err := notificationSvc.DeleteNotification(ctx, &pb.DeleteNotificationRequest{Id: parts[0]})
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
			return
		}

		http.Error(w, "Not found", http.StatusNotFound)
	})

	// Prompt Handlers
	http.HandleFunc("/api/v1/prompts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			return
		}

		switch r.Method {
		case http.MethodGet:
			q := r.URL.Query()
			req := &pb.ListPromptsRequest{
				OwnerId:    q.Get("owner_id"),
				TemplateId: q.Get("template_id"),
			}
			if v := q.Get("page_size"); v != "" {
				if i, err := strconv.Atoi(v); err == nil {
					req.PageSize = int32(i)
				}
			}
			req.PageToken = q.Get("page_token")

			resp, err := svc.ListPrompts(context.Background(), req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)

		case http.MethodPost:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read body", http.StatusBadRequest)
				return
			}

			// Accept variables as array of objects [{name: value}...] from frontend.
			var payload map[string]interface{}
			if err := json.Unmarshal(body, &payload); err != nil {
				http.Error(w, "Invalid JSON body", http.StatusBadRequest)
				return
			}

			templateID, _ := payload["template_id"].(string)
			ownerID, _ := payload["owner_id"].(string)

			var versionID int32
			if v, ok := payload["version_id"]; ok {
				switch vt := v.(type) {
				case float64:
					versionID = int32(vt)
				case int:
					versionID = int32(vt)
				}
			}

			// variables is now a map[string]string
			var variables map[string]string
			if v, ok := payload["variables"]; ok {
				if m, ok := v.(map[string]interface{}); ok {
					variables = make(map[string]string)
					for k, val := range m {
						variables[k] = fmt.Sprintf("%v", val)
					}
				}
			}

			if variables == nil {
				variables = make(map[string]string)
			}

			zap.S().Infof("CreatePrompt: incoming variables map: %v", variables)

			// Fetch template version content to render it
			version, err := templateVersionRepo.Get(context.Background(), versionID)
			if err != nil {
				writeError(w, status.Errorf(codes.NotFound, "template version not found"))
				return
			}

			// Dynamically resolve variables in the template content
			rendered := version.Content
			for k, v := range variables {
				rendered = strings.ReplaceAll(rendered, "${"+k+"}", v)
			}

			// Build prompt model and persist directly
			prompt := &models.Prompt{
				TemplateID: templateID,
				VersionID:  versionID,
				OwnerID:    ownerID,
				Variables:  variables,
				Content:    rendered,
			}

			if err := promptRepo.Create(context.Background(), prompt); err != nil {
				writeError(w, status.Errorf(codes.Internal, "failed to create prompt: %v", err))
				return
			}

			zap.S().Infof("CreatePrompt: stored prompt ID: %s", prompt.ID)

			pbPrompt := &pb.Prompt{
				Id:         prompt.ID,
				TemplateId: prompt.TemplateID,
				VersionId:  prompt.VersionID,
				OwnerId:    prompt.OwnerID,
				Variables:  prompt.Variables,
				Content:    prompt.Content,
				CreatedAt:  timestamppb.New(prompt.CreatedAt),
			}
			resp := &pb.CreatePromptResponse{Prompt: pbPrompt}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("/api/v1/prompts/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			return
		}

		id := strings.TrimPrefix(r.URL.Path, "/api/v1/prompts/")
		if id == "" {
			http.Error(w, "ID required", http.StatusBadRequest)
			return
		}

		switch r.Method {
		case http.MethodGet:
			req := &pb.GetPromptRequest{Id: id}
			resp, err := svc.GetPrompt(context.Background(), req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)

		case http.MethodDelete:
			ownerID := r.URL.Query().Get("owner_id")
			req := &pb.DeletePromptRequest{Id: id, OwnerId: ownerID}
			resp, err := svc.DeletePrompt(context.Background(), req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// ---- Admin REST handlers (/api/v1/admin/*) ----

	// requireAdmin verifies the admin token and returns an admin context.
	requireAdmin := func(w http.ResponseWriter, r *http.Request) (context.Context, bool) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return nil, false
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := authInterceptor.VerifyTokenWithClaims(tokenStr)
		if err != nil {
			writeError(w, err)
			return nil, false
		}
		if claims.Role != "admin" || claims.Scope != "admin" {
			http.Error(w, "admin privileges required", http.StatusForbidden)
			return nil, false
		}
		ctx := service.ContextWithUserID(r.Context(), claims.UserID)
		ctx = service.ContextWithUserRole(ctx, claims.Role)
		ctx = service.ContextWithUserScope(ctx, claims.Scope)
		return ctx, true
	}

	// clientIP extracts the caller IP for admin login rate limiting.
	clientIP := func(r *http.Request) string {
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			if idx := strings.Index(fwd, ","); idx > 0 {
				return strings.TrimSpace(fwd[:idx])
			}
			return strings.TrimSpace(fwd)
		}
		host := r.RemoteAddr
		if idx := strings.LastIndex(host, ":"); idx > 0 {
			host = host[:idx]
		}
		return host
	}

	// withClientIP attaches the client IP as gRPC metadata for the service.
	withClientIP := func(ctx context.Context, ip string) context.Context {
		return metadata.NewIncomingContext(ctx, metadata.Pairs("x-client-ip", ip))
	}

	http.HandleFunc("/api/v1/admin/login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusBadRequest)
			return
		}
		var req pb.AdminLoginRequest
		if err := unmarshaler.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp, err := adminSvc.AdminLogin(withClientIP(context.Background(), clientIP(r)), &req)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		b, _ := marshaler.Marshal(resp)
		_, _ = w.Write(b)
	})

	http.HandleFunc("/api/v1/admin/session/refresh", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ctx, ok := requireAdmin(w, r)
		if !ok {
			return
		}
		resp, err := adminSvc.RefreshAdminSession(ctx, &pb.RefreshAdminSessionRequest{})
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		b, _ := marshaler.Marshal(resp)
		_, _ = w.Write(b)
	})

	http.HandleFunc("/api/v1/admin/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ctx, ok := requireAdmin(w, r)
		if !ok {
			return
		}
		resp, err := adminSvc.GetAdminStats(ctx, &pb.GetAdminStatsRequest{})
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		b, _ := marshaler.Marshal(resp)
		_, _ = w.Write(b)
	})

	http.HandleFunc("/api/v1/admin/users", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ctx, ok := requireAdmin(w, r)
		if !ok {
			return
		}

		req := &pb.ListUsersRequest{}
		q := r.URL.Query()
		req.Search = q.Get("search")
		req.Status = q.Get("status")
		req.Role = q.Get("role")
		req.Sort = q.Get("sort")
		if v, err := strconv.Atoi(q.Get("page")); err == nil {
			req.Page = int32(v)
		}
		if v, err := strconv.Atoi(q.Get("per_page")); err == nil {
			req.PerPage = int32(v)
		}

		resp, err := adminSvc.ListUsers(ctx, req)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		b, _ := marshaler.Marshal(resp)
		_, _ = w.Write(b)
	})

	http.HandleFunc("/api/v1/admin/templates", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ctx, ok := requireAdmin(w, r)
		if !ok {
			return
		}

		req := &pb.ListAdminTemplatesRequest{}
		q := r.URL.Query()
		req.Search = q.Get("search")
		req.Visibility = q.Get("visibility")
		req.OwnerId = q.Get("owner_id")
		req.Sort = q.Get("sort")
		if v, err := strconv.Atoi(q.Get("page")); err == nil {
			req.Page = int32(v)
		}
		if v, err := strconv.Atoi(q.Get("per_page")); err == nil {
			req.PerPage = int32(v)
		}

		resp, err := adminSvc.ListTemplates(ctx, req)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		b, _ := marshaler.Marshal(resp)
		_, _ = w.Write(b)
	})

	http.HandleFunc("/api/v1/admin/audit-log", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ctx, ok := requireAdmin(w, r)
		if !ok {
			return
		}

		req := &pb.ListAuditLogsRequest{}
		q := r.URL.Query()
		req.Action = q.Get("action")
		req.Actor = q.Get("actor")
		if v, err := strconv.Atoi(q.Get("page")); err == nil {
			req.Page = int32(v)
		}
		if v, err := strconv.Atoi(q.Get("per_page")); err == nil {
			req.PerPage = int32(v)
		}

		resp, err := adminSvc.ListAuditLogs(ctx, req)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		b, _ := marshaler.Marshal(resp)
		_, _ = w.Write(b)
	})

	http.HandleFunc("/api/v1/admin/templates/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ctx, ok := requireAdmin(w, r)
		if !ok {
			return
		}

		id := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/templates/")
		if id == "" {
			http.Error(w, "ID required", http.StatusBadRequest)
			return
		}

		resp, err := adminSvc.GetTemplate(ctx, &pb.GetAdminTemplateRequest{Id: id})
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		b, _ := marshaler.Marshal(resp)
		_, _ = w.Write(b)
	})

	http.HandleFunc("/api/v1/admin/users/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			return
		}

		ctx, ok := requireAdmin(w, r)
		if !ok {
			return
		}

		rest := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/users/")
		parts := strings.Split(rest, "/")
		if parts[0] == "" {
			http.Error(w, "ID required", http.StatusBadRequest)
			return
		}
		userID := parts[0]

		// GET /api/v1/admin/users/{id}
		if r.Method == http.MethodGet && len(parts) == 1 {
			resp, err := adminSvc.GetUser(ctx, &pb.GetUserRequest{Id: userID})
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
			return
		}

		if r.Method != http.MethodPost || len(parts) != 2 {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusBadRequest)
			return
		}

		switch parts[1] {
		case "ban":
			var req pb.BanUserRequest
			if err := unmarshaler.Unmarshal(body, &req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			req.UserId = userID
			resp, err := adminSvc.BanUser(ctx, &req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)

		case "unban":
			var req pb.UnbanUserRequest
			if err := unmarshaler.Unmarshal(body, &req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			req.UserId = userID
			resp, err := adminSvc.UnbanUser(ctx, &req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)

		case "role":
			var req pb.ChangeUserRoleRequest
			if err := unmarshaler.Unmarshal(body, &req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			req.UserId = userID
			resp, err := adminSvc.ChangeUserRole(ctx, &req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)

		default:
			http.Error(w, "Unknown action", http.StatusNotFound)
		}
	})

	zap.S().Info("HTTP server listening at :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		zap.S().Fatalf("failed to serve http: %v", err)
	}
}
