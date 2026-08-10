import { z } from 'zod';

/**
 * Zod contracts for the Stage 13A visual-design API
 * (`internal/httpapi/visualdesign.go`): the management document shape
 * (GET/PUT/DELETE .../visual-design) and the smaller public shape
 * embedded inside a public `alert.show` payload.
 *
 * See docs/visual-designs.md for the full contract this mirrors.
 *
 * Deliberately does not import anything from `./alerts-schemas` (even
 * though `alertAnimationSchema` there has the exact same values as
 * `visualDesignAnimationSchema` below) - `alerts-schemas.ts` itself
 * imports from this file (for the public alert payload's own
 * `visualDesign`/`renderingMode` fields), and a two-way import between
 * the same two modules is a real ESM circular-import hazard (one
 * module's export can still be `undefined` when the other's top-level
 * code runs) - confirmed the hard way while writing this file's own
 * tests. One small, stable, closed enum duplicated here is simpler and
 * safer than untangling a cycle.
 */

/** The same closed animation vocabulary alerts/the chat overlay
 * already use - see visualdesign.Animation's own Go doc comment for
 * why the backend keeps it a distinct type even though the values are
 * identical. */
export const visualDesignAnimationSchema = z.enum(['none', 'fade', 'slide_up', 'slide_left', 'scale']);
export type VisualDesignAnimation = z.infer<typeof visualDesignAnimationSchema>;

export const visualDesignLayerKindSchema = z.enum(['shape', 'text', 'platform_icon', 'avatar']);
export type VisualDesignLayerKind = z.infer<typeof visualDesignLayerKindSchema>;

export const visualDesignShapeKindSchema = z.enum(['rectangle']);
export type VisualDesignShapeKind = z.infer<typeof visualDesignShapeKindSchema>;

export const visualDesignTextBindingSchema = z.enum([
  'static',
  'alert_rendered_text',
  'username',
  'platform',
  'event_type',
  'message',
  'quantity',
  'group_count',
]);
export type VisualDesignTextBinding = z.infer<typeof visualDesignTextBindingSchema>;

export const visualDesignMissingValueBehaviorSchema = z.enum(['hide', 'placeholder']);
export type VisualDesignMissingValueBehavior = z.infer<typeof visualDesignMissingValueBehaviorSchema>;

export const visualDesignFontFamilySchema = z.enum(['system-ui', 'sans-serif', 'serif', 'monospace']);
export type VisualDesignFontFamily = z.infer<typeof visualDesignFontFamilySchema>;

export const visualDesignHorizontalAlignSchema = z.enum(['left', 'center', 'right']);
export type VisualDesignHorizontalAlign = z.infer<typeof visualDesignHorizontalAlignSchema>;

export const visualDesignVerticalAlignSchema = z.enum(['top', 'middle', 'bottom']);
export type VisualDesignVerticalAlign = z.infer<typeof visualDesignVerticalAlignSchema>;

export const visualDesignFrameSchema = z.object({
  x: z.number(),
  y: z.number(),
  width: z.number(),
  height: z.number(),
});
export type VisualDesignFrame = z.infer<typeof visualDesignFrameSchema>;

export const visualDesignCanvasSchema = z.object({
  width: z.number(),
  height: z.number(),
  transparent: z.boolean(),
});
export type VisualDesignCanvas = z.infer<typeof visualDesignCanvasSchema>;

export const visualDesignShapePropsSchema = z.object({
  kind: visualDesignShapeKindSchema,
  fill: z.string(),
  borderColor: z.string(),
  borderWidth: z.number(),
  cornerRadius: z.number(),
});
export type VisualDesignShapeProps = z.infer<typeof visualDesignShapePropsSchema>;

export const visualDesignTextPropsSchema = z.object({
  binding: visualDesignTextBindingSchema,
  staticText: z.string().optional(),
  missingValueBehavior: visualDesignMissingValueBehaviorSchema,
  fontFamily: visualDesignFontFamilySchema,
  fontSize: z.number(),
  fontWeight: z.number(),
  lineHeight: z.number(),
  letterSpacing: z.number(),
  textColor: z.string(),
  horizontalAlign: visualDesignHorizontalAlignSchema,
  verticalAlign: visualDesignVerticalAlignSchema,
  outlineWidth: z.number(),
  outlineColor: z.string(),
  shadowEnabled: z.boolean(),
  shadowOffsetX: z.number(),
  shadowOffsetY: z.number(),
  shadowBlur: z.number(),
  shadowColor: z.string(),
});
export type VisualDesignTextProps = z.infer<typeof visualDesignTextPropsSchema>;

export const visualDesignAvatarPropsSchema = z.object({
  cornerRadius: z.number(),
  borderColor: z.string(),
  borderWidth: z.number(),
});
export type VisualDesignAvatarProps = z.infer<typeof visualDesignAvatarPropsSchema>;

/** The management layer shape - includes `name`/`locked`, which the
 * public shape below deliberately never carries. */
export const visualDesignLayerSchema = z.object({
  id: z.string(),
  name: z.string(),
  kind: visualDesignLayerKindSchema,
  visible: z.boolean(),
  locked: z.boolean(),
  order: z.number(),
  frame: visualDesignFrameSchema,
  opacity: z.number(),
  shape: visualDesignShapePropsSchema.optional(),
  text: visualDesignTextPropsSchema.optional(),
  platformIcon: z.object({}).optional(),
  avatar: visualDesignAvatarPropsSchema.optional(),
  entryAnimation: visualDesignAnimationSchema,
  exitAnimation: visualDesignAnimationSchema,
  animationDurationMs: z.number(),
});
export type VisualDesignLayer = z.infer<typeof visualDesignLayerSchema>;

export const visualDesignDocumentSchema = z.object({
  version: z.number(),
  canvas: visualDesignCanvasSchema,
  layers: z.array(visualDesignLayerSchema),
});
export type VisualDesignDocument = z.infer<typeof visualDesignDocumentSchema>;

/** `GET`/`PUT /api/alert-rules/{id}/visual-design` response. */
export const visualDesignResponseSchema = z.object({
  persisted: z.boolean(),
  revision: z.number(),
  document: visualDesignDocumentSchema,
});
export type VisualDesignResponse = z.infer<typeof visualDesignResponseSchema>;

// --- public shape (embedded inside a public alert.show payload) ---------

/** The public layer shape - never `name`/`locked` (Stage 13A task Part
 * 17/44: management-only fields never reach a public payload). */
export const publicVisualDesignLayerSchema = z.object({
  id: z.string(),
  kind: visualDesignLayerKindSchema,
  frame: visualDesignFrameSchema,
  opacity: z.number(),
  shape: visualDesignShapePropsSchema.optional(),
  text: visualDesignTextPropsSchema.optional(),
  platformIcon: z.object({}).optional(),
  avatar: visualDesignAvatarPropsSchema.optional(),
  entryAnimation: visualDesignAnimationSchema,
  exitAnimation: visualDesignAnimationSchema,
  animationDurationMs: z.number(),
});
export type PublicVisualDesignLayer = z.infer<typeof publicVisualDesignLayerSchema>;

export const publicVisualDesignCanvasSchema = z.object({
  width: z.number(),
  height: z.number(),
  transparent: z.boolean(),
});
export type PublicVisualDesignCanvas = z.infer<typeof publicVisualDesignCanvasSchema>;

export const publicVisualDesignDocumentSchema = z.object({
  schemaVersion: z.number(),
  canvas: publicVisualDesignCanvasSchema,
  layers: z.array(publicVisualDesignLayerSchema),
});
export type PublicVisualDesignDocument = z.infer<typeof publicVisualDesignDocumentSchema>;

/** Stage 13A's own additive, closed `PublicAlert` discriminator - see
 * internal/alerts.RenderingMode. Defaults to `"legacy"` so an alert
 * payload that somehow omits this new field (never expected from this
 * project's own backend, but a defensive default costs nothing) never
 * gets treated as design-driven. */
export const visualDesignRenderingModeSchema = z.enum(['legacy', 'visual_design']).catch('legacy');
export type VisualDesignRenderingMode = z.infer<typeof visualDesignRenderingModeSchema>;
