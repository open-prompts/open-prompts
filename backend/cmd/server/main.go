package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
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

// retryAfterRegex extracts the seconds hint from rate-limit messages like
// "rate limit exceeded, retry after 42 seconds".
var retryAfterRegex = regexp.MustCompile(`retry after (\d+) seconds`)

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
		// Generic rate-limit hint: parse trailing seconds from messages like
		// "rate limit exceeded, retry after N seconds".
		if matches := retryAfterRegex.FindStringSubmatch(st.Message()); matches != nil {
			w.Header().Set("Retry-After", matches[1])
		}
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
	commentRepo := repository.NewCommentRepository(pgConn.DB)
	followRepo := repository.NewFollowRepository(pgConn.DB)

	// User repository is needed by the prompt service for notification
	// actor lookups; construct it before the prompt service.
	userRepo := repository.NewUserRepository(pgConn.DB)

	// Redis
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	redisClient, err := data.NewRedisClient(redisAddr, "", 0)
	if err != nil {
		zap.S().Fatalf("failed to connect to redis: %v", err)
	}

	svc := service.NewPromptService(promptRepo, templateRepo, templateVersionRepo, templateAliasRepo, notificationRepo, userRepo, commentRepo, redisClient)

	// Pre-publication review: the review repository backs the gating
	// checks, the admin review queue, and platform settings.
	reviewRepo := repository.NewReviewRepository(pgConn.DB)
	svc.SetReviewRepo(reviewRepo)
	svc.SetFollowRepo(followRepo)

	// Prompt analytics: usage events + aggregation.
	analyticsRepo := repository.NewAnalyticsRepository(pgConn.DB)
	svc.SetAnalyticsRepo(analyticsRepo)

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
	userSvc.SetFollowRepo(followRepo)
	userSvc.SetNotificationRepo(notificationRepo)

	// Admin Service
	adminRepo := repository.NewAdminRepository(pgConn.DB)
	sessionTTLMinutes := 30
	if v := os.Getenv("ADMIN_SESSION_TTL_MINUTES"); v != "" {
		if parsed, perr := strconv.Atoi(v); perr == nil && parsed > 0 {
			sessionTTLMinutes = parsed
		}
	}
	adminSvc := service.NewAdminService(userRepo, adminRepo, apiKeyRepo, redisClient, jwtSecret, time.Duration(sessionTTLMinutes)*time.Minute)
	adminSvc.SetReviewRepo(reviewRepo)
	adminSvc.SetNotificationRepo(notificationRepo)
	adminSvc.SetPromptService(svc)

	// Moderation appeals: the appeal repository backs the user + admin appeal
	// RPCs and the moderation_actions reads.
	appealRepo := repository.NewAppealRepository(pgConn.DB)
	svc.SetAppealRepo(appealRepo)
	svc.SetAdminRepo(adminRepo)
	adminSvc.SetAppealRepo(appealRepo)

	// Content reporting: the report repository backs the user + admin report
	// RPCs and the auto-resolve hooks.
	reportRepo := repository.NewReportRepository(pgConn.DB)
	svc.SetReportRepo(reportRepo)
	adminSvc.SetReportRepo(reportRepo)
	adminSvc.SetCommentRepo(commentRepo)

	// Template moderation: the 30-day hard-delete ticker (admin-template-
	// moderation) reuses the shared cascade service.
	cascadeSvc := service.NewTemplateCascadeService(pgConn.DB)
	tickerSvc := service.NewTickerService(pgConn.DB, cascadeSvc)
	tickerSvc.Start(context.Background())

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

	// Collection Service (user-curated template collections)
	collectionRepo := repository.NewCollectionRepository(pgConn.DB)
	collectionSvc := service.NewCollectionService(collectionRepo, templateRepo, userRepo)

	// Dashboard Service (personal workspace overview)
	dashboardRepo := repository.NewDashboardRepository(pgConn.DB)
	dashboardSvc := service.NewDashboardService(dashboardRepo, apiKeyRepo)

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
		pb.RegisterCollectionServiceServer(s, collectionSvc)
		pb.RegisterDashboardServiceServer(s, dashboardSvc)
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
			req.Keyword = q.Get("keyword")
			if req.Keyword == "" {
				req.Keyword = q.Get("q") // alias
			}
			req.Sort = q.Get("sort")
			if v := q.Get("exclude_warned"); v == "true" {
				req.ExcludeWarned = true
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

	// clientIP extracts the caller IP for rate limiting (admin login, view
	// events). Defined before the handlers that use it.
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
					// Prompt analytics: fire-and-forget API use event. The
					// alias endpoint is public; attribute the user when a
					// valid token is presented, else record with empty id.
					authHeader := r.Header.Get("Authorization")
					tplID := templateID
					go func() {
						ctx := context.Background()
						uid := ""
						if authHeader != "" {
							parts := strings.SplitN(authHeader, " ", 2)
							if len(parts) == 2 {
								if id, err := authInterceptor.VerifyAnyToken(ctx, parts[1]); err == nil {
									uid = id
								}
							}
						}
						svc.RecordUseEvent(ctx, tplID, uid, "api")
					}()
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

			ctx := context.Background()
			if authHeader := r.Header.Get("Authorization"); authHeader != "" {
				tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
				if userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr); err == nil {
					ctx = service.ContextWithUserID(ctx, userID)
				}
			}
			resp, err := svc.ListTemplateVersions(ctx, req)
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

		if strings.HasSuffix(id, "/rating") {
			templateID := strings.TrimSuffix(id, "/rating")
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
			case http.MethodPost:
				var body struct {
					Rating int32 `json:"rating"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, "Invalid request body", http.StatusBadRequest)
					return
				}
				req := &pb.RateTemplateRequest{TemplateId: templateID, Rating: body.Rating}
				resp, err := svc.RateTemplate(ctx, req)
				if err != nil {
					writeError(w, err)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				b, _ := marshaler.Marshal(resp)
				_, _ = w.Write(b)
				return
			case http.MethodDelete:
				req := &pb.DeleteRatingRequest{TemplateId: templateID}
				resp, err := svc.DeleteRating(ctx, req)
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

		// /api/v1/templates/{template_id}/comments — GET (public read with
		// optional auth) and POST (auth required).
		if len(parts) == 2 && parts[1] == "comments" {
			templateID := parts[0]
			switch r.Method {
			case http.MethodGet:
				ctx := context.Background()
				if authHeader := r.Header.Get("Authorization"); authHeader != "" {
					tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
					if userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr); err == nil {
						ctx = service.ContextWithUserID(ctx, userID)
					}
				}
				page := 1
				if v := r.URL.Query().Get("page"); v != "" {
					if parsed, perr := strconv.Atoi(v); perr == nil && parsed > 0 {
						page = parsed
					}
				}
				size := 20
				if v := r.URL.Query().Get("page_size"); v != "" {
					if parsed, perr := strconv.Atoi(v); perr == nil && parsed > 0 {
						size = parsed
					}
				}
				req := &pb.ListCommentsRequest{TemplateId: templateID, Page: int32(page), PageSize: int32(size)}
				resp, err := svc.ListComments(ctx, req)
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
					writeError(w, err)
					return
				}
				ctx := service.ContextWithUserID(context.Background(), userID)

				var body struct {
					Content         string `json:"content"`
					ParentCommentId string `json:"parent_comment_id"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, "Invalid request body", http.StatusBadRequest)
					return
				}
				req := &pb.CreateCommentRequest{TemplateId: templateID, Content: body.Content, ParentCommentId: body.ParentCommentId}
				resp, err := svc.CreateComment(ctx, req)
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

		// /api/v1/templates/{template_id}/events — POST view beacon
		// (anonymous allowed; rate-limited per IP).
		if len(parts) == 2 && parts[1] == "events" {
			templateID := parts[0]
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			ctx := context.Background()
			if authHeader := r.Header.Get("Authorization"); authHeader != "" {
				tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
				if userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr); err == nil {
					ctx = service.ContextWithUserID(ctx, userID)
				}
			}
			var body struct {
				Type      string `json:"type"`
				SessionID string `json:"session_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "Invalid request body", http.StatusBadRequest)
				return
			}
			req := &pb.RecordTemplateEventRequest{
				TemplateId: templateID,
				EventType:  body.Type,
				SessionId:  body.SessionID,
			}
			resp, err := svc.RecordTemplateEvent(withClientIP(ctx, clientIP(r)), req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
			return
		}

		// /api/v1/templates/{template_id}/stats?days=30 — owner-only analytics.
		if len(parts) == 2 && parts[1] == "stats" {
			templateID := parts[0]
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
				writeError(w, err)
				return
			}
			ctx := service.ContextWithUserID(context.Background(), userID)

			days := 30
			if v := r.URL.Query().Get("days"); v != "" {
				if parsed, perr := strconv.Atoi(v); perr == nil {
					days = parsed
				}
			}
			req := &pb.GetTemplateStatsRequest{TemplateId: templateID, Days: int32(days)}
			resp, err := svc.GetTemplateStats(ctx, req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
			return
		}

		// /api/v1/templates/{template_id}/related — GET (public, optional
		// auth; invalid token -> 401, matching the detail GET).
		if len(parts) == 2 && parts[1] == "related" {
			templateID := parts[0]
			if r.Method != http.MethodGet {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			ctx := context.Background()
			if authHeader := r.Header.Get("Authorization"); authHeader != "" {
				tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
				if userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr); err == nil {
					ctx = service.ContextWithUserID(ctx, userID)
				} else {
					http.Error(w, "Invalid token", http.StatusUnauthorized)
					return
				}
			}
			req := &pb.GetRelatedTemplatesRequest{TemplateId: templateID}
			if v := r.URL.Query().Get("limit"); v != "" {
				if i, err := strconv.Atoi(v); err == nil {
					req.Limit = int32(i)
				}
			}
			resp, err := svc.GetRelatedTemplates(ctx, req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
			return
		}

		// /api/v1/templates/{template_id}/more-from-author — GET (public,
		// optional auth; invalid token -> 401).
		if len(parts) == 2 && parts[1] == "more-from-author" {
			templateID := parts[0]
			if r.Method != http.MethodGet {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			ctx := context.Background()
			if authHeader := r.Header.Get("Authorization"); authHeader != "" {
				tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
				if userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr); err == nil {
					ctx = service.ContextWithUserID(ctx, userID)
				} else {
					http.Error(w, "Invalid token", http.StatusUnauthorized)
					return
				}
			}
			req := &pb.ListMoreFromAuthorRequest{TemplateId: templateID}
			if v := r.URL.Query().Get("limit"); v != "" {
				if i, err := strconv.Atoi(v); err == nil {
					req.Limit = int32(i)
				}
			}
			resp, err := svc.ListMoreFromAuthor(ctx, req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
			return
		}

		// /api/v1/templates/{template_id}/collections — GET (auth) returns the
		// caller's collections containing the template (picker checkmarks).
		if len(parts) == 2 && parts[1] == "collections" {
			templateID := parts[0]
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
				writeError(w, err)
				return
			}
			ctx := service.ContextWithUserID(context.Background(), userID)
			req := &pb.ListCollectionsContainingTemplateRequest{TemplateId: templateID}
			resp, err := collectionSvc.ListCollectionsContainingTemplate(ctx, req)
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

	// /api/v1/analytics/overview?days=30 — creator analytics dashboard.
	http.HandleFunc("/api/v1/analytics/overview", func(w http.ResponseWriter, r *http.Request) {
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
			writeError(w, err)
			return
		}
		ctx := service.ContextWithUserID(context.Background(), userID)

		days := 30
		if v := r.URL.Query().Get("days"); v != "" {
			if parsed, perr := strconv.Atoi(v); perr == nil {
				days = parsed
			}
		}
		req := &pb.GetCreatorStatsRequest{Days: int32(days)}
		resp, err := svc.GetCreatorStats(ctx, req)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		b, _ := marshaler.Marshal(resp)
		_, _ = w.Write(b)
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

	// Follow Creators: /api/v1/users routes. The /me/following-feed route is
	// registered before the /{userId} patterns so "me" is never captured as a
	// userId.
	http.HandleFunc("/api/v1/users/me/following-feed", func(w http.ResponseWriter, r *http.Request) {
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

		req := &pb.GetFollowingFeedRequest{}
		q := r.URL.Query()
		if v := q.Get("page_size"); v != "" {
			if i, err := strconv.Atoi(v); err == nil {
				req.PageSize = int32(i)
			}
		}
		req.PageToken = q.Get("page_token")

		resp, err := userSvc.GetFollowingFeed(ctx, req)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		b, _ := marshaler.Marshal(resp)
		_, _ = w.Write(b)
	})

	// GET /api/v1/users?sort=followers&limit=5 — top creators by followers
	// (public user-card shape), used for empty-feed suggestions.
	http.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
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

		ctx := context.Background()
		if authHeader := r.Header.Get("Authorization"); authHeader != "" {
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			if userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr); err == nil {
				ctx = service.ContextWithUserID(ctx, userID)
			}
		}

		q := r.URL.Query()
		if q.Get("sort") == "followers" {
			req := &pb.ListTopCreatorsRequest{}
			if v := q.Get("limit"); v != "" {
				if i, err := strconv.Atoi(v); err == nil {
					req.Limit = int32(i)
				}
			}
			resp, err := userSvc.ListTopCreators(ctx, req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
			return
		}
		http.Error(w, "unsupported query", http.StatusBadRequest)
	})

	// /api/v1/users/{userId}/... — follow, profile, templates, followers, following.
	http.HandleFunc("/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			return
		}

		rest := strings.TrimPrefix(r.URL.Path, "/api/v1/users/")
		parts := strings.Split(rest, "/")
		userID := parts[0]
		if userID == "" {
			http.Error(w, "user id required", http.StatusBadRequest)
			return
		}

		// Optional auth for public reads; required for follow/unfollow.
		ctx := context.Background()
		authedUserID := ""
		authErr := error(nil)
		if authHeader := r.Header.Get("Authorization"); authHeader != "" {
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			if id, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr); err == nil {
				ctx = service.ContextWithUserID(ctx, id)
				authedUserID = id
			} else {
				authErr = err
			}
		}

		// /api/v1/users/{userId}/follow
		if len(parts) == 2 && parts[1] == "follow" {
			if authErr != nil {
				// Banned users' tokens are rejected by the auth layer with
				// PermissionDenied; propagate it so the client sees 403.
				writeError(w, authErr)
				return
			}
			if authedUserID == "" {
				http.Error(w, "Authorization header required", http.StatusUnauthorized)
				return
			}
			req := &pb.FollowUserRequest{UserId: userID, FollowerId: authedUserID}
			var resp *pb.FollowUserResponse
			var err error
			switch r.Method {
			case http.MethodPost:
				resp, err = userSvc.FollowUser(ctx, req)
			case http.MethodDelete:
				resp, err = userSvc.UnfollowUser(ctx, req)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
			return
		}

		// /api/v1/users/{userId}/profile
		if len(parts) == 2 && parts[1] == "profile" {
			if r.Method != http.MethodGet {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			req := &pb.GetCreatorProfileRequest{UserId: userID, ViewerId: authedUserID}
			resp, err := userSvc.GetCreatorProfile(ctx, req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
			return
		}

		// /api/v1/users/{userId}/templates — public templates of the creator.
		if len(parts) == 2 && parts[1] == "templates" {
			if r.Method != http.MethodGet {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			req := &pb.ListTemplatesRequest{OwnerId: userID, Visibility: pb.Visibility_VISIBILITY_PUBLIC}
			q := r.URL.Query()
			if v := q.Get("page_size"); v != "" {
				if i, err := strconv.Atoi(v); err == nil {
					req.PageSize = int32(i)
				}
			}
			req.PageToken = q.Get("page_token")
			resp, err := svc.ListTemplates(ctx, req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
			return
		}

		// /api/v1/users/{userId}/followers
		if len(parts) == 2 && parts[1] == "followers" {
			if r.Method != http.MethodGet {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			req := &pb.ListUserRelationsRequest{UserId: userID}
			q := r.URL.Query()
			if v := q.Get("page"); v != "" {
				if i, err := strconv.Atoi(v); err == nil {
					req.Page = int32(i)
				}
			}
			if v := q.Get("page_size"); v != "" {
				if i, err := strconv.Atoi(v); err == nil {
					req.PageSize = int32(i)
				}
			}
			resp, err := userSvc.ListFollowers(ctx, req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
			return
		}

		// /api/v1/users/{userId}/following
		if len(parts) == 2 && parts[1] == "following" {
			if r.Method != http.MethodGet {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			req := &pb.ListUserRelationsRequest{UserId: userID}
			q := r.URL.Query()
			if v := q.Get("page"); v != "" {
				if i, err := strconv.Atoi(v); err == nil {
					req.Page = int32(i)
				}
			}
			if v := q.Get("page_size"); v != "" {
				if i, err := strconv.Atoi(v); err == nil {
					req.PageSize = int32(i)
				}
			}
			resp, err := userSvc.ListFollowing(ctx, req)
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

	// Moderation Appeals — /api/v1/appeals (POST create, GET list) and
	// /api/v1/appeals/{id} (GET detail, owner-only).
	http.HandleFunc("/api/v1/appeals", func(w http.ResponseWriter, r *http.Request) {
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
		case http.MethodPost:
			var req pb.CreateAppealRequest
			body, berr := io.ReadAll(r.Body)
			if berr != nil {
				http.Error(w, "Failed to read body", http.StatusBadRequest)
				return
			}
			if err := unmarshaler.Unmarshal(body, &req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			resp, err := svc.CreateAppeal(ctx, &req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
		case http.MethodGet:
			q := r.URL.Query()
			req := &pb.ListAppealsRequest{}
			if v := q.Get("page"); v != "" {
				if i, cerr := strconv.Atoi(v); cerr == nil {
					req.Page = int32(i)
				}
			}
			if v := q.Get("per_page"); v != "" {
				if i, cerr := strconv.Atoi(v); cerr == nil {
					req.PerPage = int32(i)
				}
			}
			resp, err := svc.ListAppeals(ctx, req)
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

	http.HandleFunc("/api/v1/appeals/", func(w http.ResponseWriter, r *http.Request) {
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

		id := strings.TrimPrefix(r.URL.Path, "/api/v1/appeals/")
		if id == "" {
			http.Error(w, "ID required", http.StatusBadRequest)
			return
		}
		resp, err := svc.GetAppeal(ctx, &pb.GetAppealRequest{AppealId: id})
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		b, _ := marshaler.Marshal(resp)
		_, _ = w.Write(b)
	})

	// Content Reporting (user) — /api/v1/reports (POST create/update) and
	// /api/v1/reports/mine (GET checkmark state, DELETE withdraw).
	http.HandleFunc("/api/v1/reports", func(w http.ResponseWriter, r *http.Request) {
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

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr)
		if err != nil {
			writeError(w, err)
			return
		}
		ctx := service.ContextWithUserID(context.Background(), userID)

		body, berr := io.ReadAll(r.Body)
		if berr != nil {
			http.Error(w, "Failed to read body", http.StatusBadRequest)
			return
		}
		var req pb.CreateReportRequest
		if err := unmarshaler.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp, err := svc.CreateReport(ctx, &req)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		b, _ := marshaler.Marshal(resp)
		_, _ = w.Write(b)
	})

	http.HandleFunc("/api/v1/reports/mine", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, DELETE, OPTIONS")
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
			writeError(w, err)
			return
		}
		ctx := service.ContextWithUserID(context.Background(), userID)

		switch r.Method {
		case http.MethodGet:
			q := r.URL.Query()
			req := &pb.GetMyReportRequest{ItemType: q.Get("item_type"), ItemId: q.Get("item_id")}
			resp, err := svc.GetMyReport(ctx, req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
		case http.MethodDelete:
			body, berr := io.ReadAll(r.Body)
			if berr != nil {
				http.Error(w, "Failed to read body", http.StatusBadRequest)
				return
			}
			var req pb.WithdrawReportRequest
			if err := unmarshaler.Unmarshal(body, &req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			resp, err := svc.WithdrawReport(ctx, &req)
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

	// Collection Handlers — /api/v1/collections (list/create) and
	// /api/v1/collections/... (mine, {id}, {id}/templates, ...).
	http.HandleFunc("/api/v1/dashboard/overview", func(w http.ResponseWriter, r *http.Request) {
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

		// Required auth: identity is derived from the token only.
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr)
		if err != nil {
			writeError(w, err)
			return
		}
		ctx := service.ContextWithUserID(context.Background(), userID)

		resp, err := dashboardSvc.GetDashboardOverview(ctx, &pb.GetDashboardOverviewRequest{})
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		b, _ := marshaler.Marshal(resp)
		_, _ = w.Write(b)
	})

	http.HandleFunc("/api/v1/collections", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			return
		}

		switch r.Method {
		case http.MethodGet:
			// GET /api/v1/collections — public explore feed (anonymous).
			req := &pb.ListPublicCollectionsRequest{}
			q := r.URL.Query()
			if v := q.Get("page_size"); v != "" {
				if i, cerr := strconv.Atoi(v); cerr == nil {
					req.PageSize = int32(i)
				}
			}
			req.PageToken = q.Get("page_token")
			req.Sort = q.Get("sort")
			resp, err := collectionSvc.ListPublicCollections(context.Background(), req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
		case http.MethodPost:
			// POST /api/v1/collections — create (auth).
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Authorization header required", http.StatusUnauthorized)
				return
			}
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr)
			if err != nil {
				writeError(w, err)
				return
			}
			ctx := service.ContextWithUserID(context.Background(), userID)

			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read body", http.StatusBadRequest)
				return
			}
			var req pb.CreateCollectionRequest
			if err := unmarshaler.Unmarshal(body, &req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			resp, err := collectionSvc.CreateCollection(ctx, &req)
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

	http.HandleFunc("/api/v1/collections/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			return
		}

		rest := strings.TrimPrefix(r.URL.Path, "/api/v1/collections/")
		parts := strings.Split(rest, "/")

		// /api/v1/collections/mine (GET) — matched before {id}.
		if len(parts) == 1 && parts[0] == "mine" {
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
				writeError(w, err)
				return
			}
			ctx := service.ContextWithUserID(context.Background(), userID)
			resp, err := collectionSvc.ListMyCollections(ctx, &pb.ListMyCollectionsRequest{})
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
			return
		}

		// /api/v1/collections/{id}/templates/{template_id} (DELETE)
		if len(parts) == 3 && parts[1] == "templates" {
			if r.Method != http.MethodDelete {
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
				writeError(w, err)
				return
			}
			ctx := service.ContextWithUserID(context.Background(), userID)
			req := &pb.RemoveTemplateFromCollectionRequest{
				CollectionId: parts[0],
				TemplateId:   parts[2],
			}
			resp, err := collectionSvc.RemoveTemplateFromCollection(ctx, req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
			return
		}

		// /api/v1/collections/{id}/templates (POST add, PUT reorder)
		if len(parts) == 2 && parts[1] == "templates" {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Authorization header required", http.StatusUnauthorized)
				return
			}
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr)
			if err != nil {
				writeError(w, err)
				return
			}
			ctx := service.ContextWithUserID(context.Background(), userID)

			switch r.Method {
			case http.MethodPost:
				var body struct {
					TemplateId string `json:"template_id"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, "Invalid request body", http.StatusBadRequest)
					return
				}
				req := &pb.AddTemplateToCollectionRequest{
					CollectionId: parts[0],
					TemplateId:   body.TemplateId,
				}
				resp, err := collectionSvc.AddTemplateToCollection(ctx, req)
				if err != nil {
					writeError(w, err)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				b, _ := marshaler.Marshal(resp)
				_, _ = w.Write(b)
			case http.MethodPut:
				var body struct {
					TemplateIds []string `json:"template_ids"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, "Invalid request body", http.StatusBadRequest)
					return
				}
				req := &pb.ReorderCollectionTemplatesRequest{
					CollectionId: parts[0],
					TemplateIds:  body.TemplateIds,
				}
				resp, err := collectionSvc.ReorderCollectionTemplates(ctx, req)
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
			return
		}

		// /api/v1/collections/{id} (GET anonymous, PATCH update, DELETE)
		if len(parts) == 1 && parts[0] != "" {
			collectionID := parts[0]
			switch r.Method {
			case http.MethodGet:
				ctx := context.Background()
				if authHeader := r.Header.Get("Authorization"); authHeader != "" {
					tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
					if userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr); err == nil {
						ctx = service.ContextWithUserID(ctx, userID)
					}
				}
				resp, err := collectionSvc.GetCollection(ctx, &pb.GetCollectionRequest{Id: collectionID})
				if err != nil {
					writeError(w, err)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				b, _ := marshaler.Marshal(resp)
				_, _ = w.Write(b)
			case http.MethodPatch:
				authHeader := r.Header.Get("Authorization")
				if authHeader == "" {
					http.Error(w, "Authorization header required", http.StatusUnauthorized)
					return
				}
				tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
				userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr)
				if err != nil {
					writeError(w, err)
					return
				}
				ctx := service.ContextWithUserID(context.Background(), userID)
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, "Failed to read body", http.StatusBadRequest)
					return
				}
				var req pb.UpdateCollectionRequest
				if err := unmarshaler.Unmarshal(body, &req); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				req.Id = collectionID
				resp, err := collectionSvc.UpdateCollection(ctx, &req)
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
					writeError(w, err)
					return
				}
				ctx := service.ContextWithUserID(context.Background(), userID)
				resp, err := collectionSvc.DeleteCollection(ctx, &pb.DeleteCollectionRequest{Id: collectionID})
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
			return
		}

		http.Error(w, "Not found", http.StatusNotFound)
	})

	// Comment Handlers — /api/v1/comments/{id} (PUT edit, DELETE author-or-owner)
	// and /api/v1/comments/{id}/replies (GET list replies, optional auth).
	http.HandleFunc("/api/v1/comments/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			return
		}

		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/comments/"), "/")
		if len(parts) == 2 && parts[1] == "replies" && r.Method == http.MethodGet {
			// Optional auth — visibility follows the template.
			ctx := context.Background()
			if authHeader := r.Header.Get("Authorization"); authHeader != "" {
				tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
				if userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr); err == nil {
					ctx = service.ContextWithUserID(ctx, userID)
				}
			}
			resp, err := svc.ListCommentReplies(ctx, &pb.ListCommentRepliesRequest{CommentId: parts[0]})
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
			return
		}

		commentID := parts[0]
		if commentID == "" || len(parts) != 1 {
			http.Error(w, "ID required", http.StatusBadRequest)
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
			writeError(w, err)
			return
		}
		ctx := service.ContextWithUserID(context.Background(), userID)

		switch r.Method {
		case http.MethodPut:
			var body struct {
				Content string `json:"content"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "Invalid request body", http.StatusBadRequest)
				return
			}
			resp, err := svc.UpdateComment(ctx, &pb.UpdateCommentRequest{CommentId: commentID, Content: body.Content})
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
		case http.MethodDelete:
			resp, err := svc.DeleteComment(ctx, &pb.DeleteCommentRequest{CommentId: commentID})
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
			// Optional auth: propagate identity so the service can enforce
			// owner-only access when owner_id is specified.
			ctx := context.Background()
			if authHeader := r.Header.Get("Authorization"); authHeader != "" {
				tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
				userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr)
				if err != nil {
					http.Error(w, "Invalid token", http.StatusUnauthorized)
					return
				}
				ctx = service.ContextWithUserID(ctx, userID)
			}

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

			resp, err := svc.ListPrompts(ctx, req)
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

			// Accept variables as array of objects [{name: value}...] from frontend.
			var payload map[string]interface{}
			if err := json.Unmarshal(body, &payload); err != nil {
				http.Error(w, "Invalid JSON body", http.StatusBadRequest)
				return
			}

			templateID, _ := payload["template_id"].(string)
			// Owner is derived from the authenticated token; the payload
			// owner_id is ignored to prevent spoofing.
			ownerID := userID

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

			if err := promptRepo.Create(ctx, prompt); err != nil {
				writeError(w, status.Errorf(codes.Internal, "failed to create prompt: %v", err))
				return
			}

			// Prompt analytics: fire-and-forget web use event. Capture the
			// values before the goroutine; never touch the response path.
			tplID, uid := prompt.TemplateID, prompt.OwnerID
			go func() {
				svc.RecordUseEvent(context.Background(), tplID, uid, "web")
			}()

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

			req := &pb.DeletePromptRequest{Id: id}
			resp, err := svc.DeletePrompt(ctx, req)
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
		req.Warning = q.Get("warning")
		req.ModerationStatus = q.Get("moderation_status")
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

	// Admin Comment Moderation — /api/v1/admin/comments (GET list)
	http.HandleFunc("/api/v1/admin/comments", func(w http.ResponseWriter, r *http.Request) {
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

		req := &pb.ListAdminCommentsRequest{}
		q := r.URL.Query()
		req.Q = q.Get("q")
		req.TemplateId = q.Get("template_id")
		req.UserId = q.Get("user_id")
		if v, err := strconv.Atoi(q.Get("page")); err == nil {
			req.Page = int32(v)
		}
		if v, err := strconv.Atoi(q.Get("per_page")); err == nil {
			req.PerPage = int32(v)
		}

		resp, err := adminSvc.ListAdminComments(ctx, req)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		b, _ := marshaler.Marshal(resp)
		_, _ = w.Write(b)
	})

	// Admin Comment Moderation — /api/v1/admin/comments/{id} (DELETE with reason)
	http.HandleFunc("/api/v1/admin/comments/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			return
		}
		if r.Method != http.MethodDelete {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ctx, ok := requireAdmin(w, r)
		if !ok {
			return
		}

		commentID := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/comments/")
		if commentID == "" || strings.Contains(commentID, "/") {
			http.Error(w, "ID required", http.StatusBadRequest)
			return
		}

		var body struct {
			Reason string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		resp, err := adminSvc.DeleteAdminComment(ctx, &pb.DeleteAdminCommentRequest{CommentId: commentID, Reason: body.Reason})
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
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			return
		}
		ctx, ok := requireAdmin(w, r)
		if !ok {
			return
		}

		rest := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/templates/")
		parts := strings.Split(rest, "/")
		if parts[0] == "" {
			http.Error(w, "ID required", http.StatusBadRequest)
			return
		}
		templateID := parts[0]

		// GET /api/v1/admin/templates/{id}
		if r.Method == http.MethodGet && len(parts) == 1 {
			resp, err := adminSvc.GetTemplate(ctx, &pb.GetAdminTemplateRequest{Id: templateID})
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
			return
		}

		// Warning routes:
		//   POST   /api/v1/admin/templates/{id}/warnings   (add)
		//   DELETE /api/v1/admin/templates/{id}/warnings   (remove)
		//   GET    /api/v1/admin/templates/{id}/warnings/history
		if len(parts) == 2 && parts[1] == "warnings" {
			if r.Method != http.MethodPost && r.Method != http.MethodDelete {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var body struct {
				Add    []string `json:"add"`
				Remove []string `json:"remove"`
				Reason string   `json:"reason"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "Invalid request body", http.StatusBadRequest)
				return
			}
			resp, err := adminSvc.UpdateTemplateWarnings(ctx, &pb.UpdateTemplateWarningsRequest{
				TemplateId: templateID,
				Add:        body.Add,
				Remove:     body.Remove,
				Reason:     body.Reason,
			})
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
			return
		}

		if len(parts) == 3 && parts[1] == "warnings" && parts[2] == "history" && r.Method == http.MethodGet {
			req := &pb.ListWarningHistoryRequest{TemplateId: templateID}
			q := r.URL.Query()
			if v, err := strconv.Atoi(q.Get("page")); err == nil {
				req.Page = int32(v)
			}
			if v, err := strconv.Atoi(q.Get("per_page")); err == nil {
				req.PerPage = int32(v)
			}
			resp, err := adminSvc.ListWarningHistory(ctx, req)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
			return
		}

		// Moderation actions:
		//   POST /api/v1/admin/templates/{id}/unpublish
		//   POST /api/v1/admin/templates/{id}/lock
		//   POST /api/v1/admin/templates/{id}/delete
		//   POST /api/v1/admin/templates/{id}/restore
		if len(parts) == 2 && r.Method == http.MethodPost {
			var body struct {
				Reason string `json:"reason"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "Invalid request body", http.StatusBadRequest)
				return
			}
			var resp *pb.ModerationActionResponse
			var err error
			switch parts[1] {
			case "unpublish":
				resp, err = adminSvc.UnpublishTemplate(ctx, &pb.ModerationActionRequest{TemplateId: templateID, Reason: body.Reason})
			case "lock":
				resp, err = adminSvc.LockTemplate(ctx, &pb.ModerationActionRequest{TemplateId: templateID, Reason: body.Reason})
			case "delete":
				resp, err = adminSvc.SoftDeleteTemplate(ctx, &pb.ModerationActionRequest{TemplateId: templateID, Reason: body.Reason})
			case "restore":
				resp, err = adminSvc.RestoreTemplate(ctx, &pb.ModerationActionRequest{TemplateId: templateID, Reason: body.Reason})
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
			return
		}

		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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

	// Pre-publication Review — /api/v1/admin/review (GET queue list)
	http.HandleFunc("/api/v1/admin/review", func(w http.ResponseWriter, r *http.Request) {
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

		req := &pb.ListReviewQueueRequest{
			Category:         r.URL.Query().Get("category"),
			AgingOnly:        r.URL.Query().Get("aging") == "true",
			ResubmissionOnly: r.URL.Query().Get("resubmission_only") == "true",
		}
		if v, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil {
			req.Page = int32(v)
		}
		if v, err := strconv.Atoi(r.URL.Query().Get("per_page")); err == nil {
			req.PerPage = int32(v)
		}
		resp, err := adminSvc.ListReviewQueue(ctx, req)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		b, _ := marshaler.Marshal(resp)
		_, _ = w.Write(b)
	})

	// Pre-publication Review — /api/v1/admin/review/{id}, /api/v1/admin/review/metrics,
	// /api/v1/admin/review/{id}/approve, /api/v1/admin/review/{id}/reject
	http.HandleFunc("/api/v1/admin/review/", func(w http.ResponseWriter, r *http.Request) {
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

		rest := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/review/")
		parts := strings.Split(rest, "/")
		if parts[0] == "" {
			http.Error(w, "ID required", http.StatusBadRequest)
			return
		}

		// GET /api/v1/admin/review/metrics
		if r.Method == http.MethodGet && parts[0] == "metrics" {
			resp, err := adminSvc.GetReviewMetrics(ctx, &pb.GetReviewMetricsRequest{})
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
			return
		}

		// GET /api/v1/admin/review/{id}
		if r.Method == http.MethodGet && len(parts) == 1 {
			resp, err := adminSvc.GetReviewSubmission(ctx, &pb.GetReviewSubmissionRequest{Id: parts[0]})
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
		case "approve":
			resp, err := adminSvc.ApproveReviewSubmission(ctx, &pb.ApproveReviewSubmissionRequest{Id: parts[0]})
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)

		case "reject":
			var req pb.RejectReviewSubmissionRequest
			if err := unmarshaler.Unmarshal(body, &req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			req.Id = parts[0]
			resp, err := adminSvc.RejectReviewSubmission(ctx, &req)
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

	// Platform Settings — /api/v1/admin/settings (GET list, PUT update)
	http.HandleFunc("/api/v1/admin/settings", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			return
		}
		ctx, ok := requireAdmin(w, r)
		if !ok {
			return
		}

		if r.Method == http.MethodGet {
			resp, err := adminSvc.GetPlatformSettings(ctx, &pb.GetPlatformSettingsRequest{})
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
			return
		}

		if r.Method != http.MethodPut {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req pb.UpdatePlatformSettingRequest
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusBadRequest)
			return
		}
		if err := unmarshaler.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp, err := adminSvc.UpdatePlatformSetting(ctx, &req)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		b, _ := marshaler.Marshal(resp)
		_, _ = w.Write(b)
	})

	// Moderation Appeals (admin) — /api/v1/admin/appeals (GET queue) and
	// /api/v1/admin/appeals/{id} (GET detail, POST review/resolve).
	http.HandleFunc("/api/v1/admin/appeals", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			return
		}
		ctx, ok := requireAdmin(w, r)
		if !ok {
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		q := r.URL.Query()
		req := &pb.ListAppealQueueRequest{State: q.Get("state")}
		if v := q.Get("page"); v != "" {
			if i, cerr := strconv.Atoi(v); cerr == nil {
				req.Page = int32(i)
			}
		}
		if v := q.Get("per_page"); v != "" {
			if i, cerr := strconv.Atoi(v); cerr == nil {
				req.PerPage = int32(i)
			}
		}
		resp, err := adminSvc.ListAppealQueue(ctx, req)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		b, _ := marshaler.Marshal(resp)
		_, _ = w.Write(b)
	})

	http.HandleFunc("/api/v1/admin/appeals/", func(w http.ResponseWriter, r *http.Request) {
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

		rest := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/appeals/")
		parts := strings.Split(rest, "/")
		if parts[0] == "" {
			http.Error(w, "ID required", http.StatusBadRequest)
			return
		}

		// GET /api/v1/admin/appeals/{id}
		if r.Method == http.MethodGet && len(parts) == 1 {
			resp, err := adminSvc.GetAppeal(ctx, &pb.GetAdminAppealRequest{AppealId: parts[0]})
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
		case "review":
			resp, err := adminSvc.ReviewAppeal(ctx, &pb.ReviewAppealRequest{AppealId: parts[0]})
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
		case "resolve":
			var req pb.ResolveAppealRequest
			if err := unmarshaler.Unmarshal(body, &req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			req.AppealId = parts[0]
			resp, err := adminSvc.ResolveAppeal(ctx, &req)
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

	// Content Reporting (admin) — /api/v1/admin/reports (GET queue) and
	// /api/v1/admin/reports/{item_type}/{item_id} (GET detail, POST resolve).
	http.HandleFunc("/api/v1/admin/reports", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			return
		}
		ctx, ok := requireAdmin(w, r)
		if !ok {
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		q := r.URL.Query()
		req := &pb.ListReportQueueRequest{
			Status:   q.Get("status"),
			ItemType: q.Get("item_type"),
			Reason:   q.Get("reason"),
		}
		if v := q.Get("page"); v != "" {
			if i, cerr := strconv.Atoi(v); cerr == nil {
				req.Page = int32(i)
			}
		}
		if v := q.Get("per_page"); v != "" {
			if i, cerr := strconv.Atoi(v); cerr == nil {
				req.PerPage = int32(i)
			}
		}
		resp, err := adminSvc.ListReportQueue(ctx, req)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		b, _ := marshaler.Marshal(resp)
		_, _ = w.Write(b)
	})

	http.HandleFunc("/api/v1/admin/reports/", func(w http.ResponseWriter, r *http.Request) {
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

		rest := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/reports/")
		parts := strings.Split(rest, "/")
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		itemType, itemID := parts[0], parts[1]

		switch r.Method {
		case http.MethodGet:
			if len(parts) != 2 {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			resp, err := adminSvc.GetReportDetail(ctx, &pb.GetReportDetailRequest{ItemType: itemType, ItemId: itemID})
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			b, _ := marshaler.Marshal(resp)
			_, _ = w.Write(b)
		case http.MethodPost:
			// POST /api/v1/admin/reports/{item_type}/{item_id}/resolve
			if len(parts) != 3 || parts[2] != "resolve" {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			body, berr := io.ReadAll(r.Body)
			if berr != nil {
				http.Error(w, "Failed to read body", http.StatusBadRequest)
				return
			}
			var req pb.ResolveReportGroupRequest
			if err := unmarshaler.Unmarshal(body, &req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			req.ItemType = itemType
			req.ItemId = itemID
			resp, err := adminSvc.ResolveReportGroup(ctx, &req)
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

	zap.S().Info("HTTP server listening at :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		zap.S().Fatalf("failed to serve http: %v", err)
	}
}
