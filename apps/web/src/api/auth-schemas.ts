import { z } from 'zod';

/**
 * Mirrors internal/httpapi/auth_routes.go's sessionBootstrapResponse -
 * the one shape both GET /api/auth/session and a successful POST
 * /api/auth/login return.
 */
export const sessionBootstrapSchema = z.object({
  authenticated: z.boolean(),
  csrfToken: z.string().optional(),
});

export type SessionBootstrap = z.infer<typeof sessionBootstrapSchema>;
