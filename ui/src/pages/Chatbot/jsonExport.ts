// Builds a comprehensive JSON export of a chat-debug conversation,
// including full context (user, org, role), all debug artifacts, Vega specs,
// CSV data, and db field annotations. Only used on the /chat-debug surface.

import type { UserInfo, OrgInfo } from '../../api/auth';
import type { Organization } from '../../store/Organizations/types';
import type { ChatChart, ChatFile, ChartContext } from '../../api/chatbot';
import type { ConversationDetail } from '../../api/chatConversations';

// ---------------------------------------------------------------------------
// Export schema types
// ---------------------------------------------------------------------------

interface DbFieldRef {
  db_table: string;
  db_fields: string[];
}

interface ExportedUser extends DbFieldRef {
  user_id: number;
  user_email: string;
  user_first_name: string | null;
  user_last_name: string | null;
  user_full_name: string;
}

interface ExportedOrg extends DbFieldRef {
  org_id: number;
  org_name: string;
  org_description: string | null;
  org_is_active: boolean;
  subscription_plan: string | null;
  subscription_status: string | null;
  max_users: number | null;
  plan_type: string | null;
}

interface ExportedRole extends DbFieldRef {
  role: string;
  is_super_admin: boolean;
  is_owner: boolean;
  is_member: boolean;
  role_lookup: string;
}

interface ExportMetadata {
  format_version: string;
  schema: string;
  exported_at: string;
  exported_by: ExportedUser;
  organization: ExportedOrg;
  user_role: ExportedRole;
}

interface ExportedConversation extends DbFieldRef {
  id: number;
  title: string;
  surface: string;
  created_at: string;
  updated_at: string;
}

interface ExportedChart extends DbFieldRef {
  title: string | undefined;
  description: string | undefined;
  query: string | undefined;
  time_range: string | undefined;
  granularity: string | undefined;
  context: ChartContext | undefined;
  vega_spec: Record<string, unknown>;
  raw_llm_output: string | undefined;
}

interface ExportedFile extends DbFieldRef {
  kind: string;
  filename: string;
  title: string | undefined;
  description: string | undefined;
  query: string | undefined;
  time_range: string | undefined;
  granularity: string | undefined;
  context: ChartContext | undefined;
  rows: number | undefined;
  csv_data: string | undefined;
}

interface ExportedTurn extends DbFieldRef {
  seq: number;
  role: string;
  text: string;
  created_at: string;
  charts: ExportedChart[];
  files: ExportedFile[];
  debug_artifacts: unknown;
}

export interface ChatDebugExport {
  export_metadata: ExportMetadata;
  conversation: ExportedConversation;
  turns: ExportedTurn[];
}

// ---------------------------------------------------------------------------
// DebugArtifacts — mirrors the interface in ChatConversation.tsx
// ---------------------------------------------------------------------------

interface DebugArtifactsResult {
  index: number;
  title: string;
  chart_type: string;
  sql: string;
  status: string;
  skip_reason?: string;
  row_count: number;
  stats?: string[];
  csv_data?: string;
  vega_spec?: string;
}

interface DebugArtifacts {
  query: string;
  schema_context: string;
  system_prompt: string;
  llm_raw_response: string;
  full_request: string;
  interpretations: Array<{
    sql: string;
    chart_type: string;
    title: string;
    description: string;
    encoding?: Record<string, unknown>;
  }>;
  results: DebugArtifactsResult[];
}

// ---------------------------------------------------------------------------
// ChatEntry — mirrors the interface in ChatConversation.tsx
// ---------------------------------------------------------------------------

interface ChatEntry {
  id: string;
  role: 'user' | 'assistant';
  text: string;
  charts?: ChatChart[];
  files?: ChatFile[];
  debugArtifacts?: DebugArtifacts | null;
}

// ---------------------------------------------------------------------------
// buildJsonExport
// ---------------------------------------------------------------------------

export function buildJsonExport(
  messages: ChatEntry[],
  user: UserInfo | null,
  organizations: OrgInfo[],
  currentOrgId: number | null,
  currentOrg: Organization | null,
  conversationDetail: ConversationDetail | undefined,
): ChatDebugExport {
  const now = new Date().toISOString();

  // Resolve role from the org list (OrgInfo has role per-org)
  const orgRole = organizations.find((o) => o.id === currentOrgId)?.role ?? 'member';
  const isSuperAdmin = orgRole === 'super_admin';
  const isOwner = orgRole === 'owner';
  const isMember = !isSuperAdmin && !isOwner;

  const metadata: ExportMetadata = {
    format_version: '1.0.0',
    schema: 'livereview/chat-debug-export/v1',
    exported_at: now,
    exported_by: {
      user_id: user?.id ?? 0,
      user_email: user?.email ?? '',
      user_first_name: user?.name?.split(' ')[0] ?? null,
      user_last_name: user?.name?.split(' ').slice(1).join(' ') || null,
      user_full_name: user?.name ?? user?.email ?? '',
      db_table: 'users',
      db_fields: ['id', 'email', 'first_name', 'last_name', 'is_active', 'created_at', 'updated_at'],
    },
    organization: {
      org_id: currentOrgId ?? 0,
      org_name: currentOrg?.name ?? '',
      org_description: currentOrg?.description ?? null,
      org_is_active: currentOrg?.is_active ?? true,
      subscription_plan: currentOrg?.subscription_plan ?? null,
      subscription_status: currentOrg?.subscription_status ?? null,
      max_users: currentOrg?.max_users ?? null,
      plan_type: currentOrg?.plan_type ?? null,
      db_table: 'organizations',
      db_fields: [
        'id', 'name', 'description', 'is_active', 'subscription_plan',
        'subscription_status', 'max_users', 'created_by_user_id', 'settings',
      ],
    },
    user_role: {
      role: orgRole,
      is_super_admin: isSuperAdmin,
      is_owner: isOwner,
      is_member: isMember,
      db_table: 'user_roles',
      db_fields: ['user_id', 'role_id', 'org_id'],
      role_lookup: 'roles.name via user_roles.role_id = roles.id',
    },
  };

  const conversation: ExportedConversation = {
    id: conversationDetail?.id ?? 0,
    title: conversationDetail?.title ?? '',
    surface: 'chat_debug',
    created_at: conversationDetail?.updatedAt ?? now,
    updated_at: conversationDetail?.updatedAt ?? now,
    db_table: 'chat_conversations',
    db_fields: ['id', 'org_id', 'user_id', 'title', 'session_id', 'surface', 'created_at', 'updated_at'],
  };

  const turns: ExportedTurn[] = messages.map((msg, i) => {
    const charts: ExportedChart[] = (msg.charts ?? []).map((ch) => ({
      title: ch.title,
      description: ch.description,
      query: ch.query,
      time_range: ch.time_range,
      granularity: ch.granularity,
      context: ch.context,
      vega_spec: ch.spec,
      raw_llm_output: undefined as string | undefined, // not available on ChatChart; kept in debug_artifacts
      db_table: 'chat_charts',
      db_fields: [
        'id', 'message_id', 'title', 'description', 'query', 'time_range',
        'granularity', 'context', 'triggering_user_message', 'vega_spec', 'raw_llm_output', 'created_at',
      ],
    }));

    const files: ExportedFile[] = (msg.files ?? []).map((f) => ({
      kind: 'csv',
      filename: f.filename,
      title: f.title,
      description: f.description,
      query: f.query,
      time_range: f.time_range,
      granularity: f.granularity,
      context: f.context,
      rows: f.rows,
      csv_data: undefined as string | undefined, // populated below from debug artifacts if available
      db_table: 'chat_files',
      db_fields: [
        'id', 'message_id', 'kind', 'filename', 'title', 'description',
        'query', 'time_range', 'granularity', 'context', 'rows', 'created_at',
      ],
    }));

    // Try to match CSV data from debug_artifacts results to files by index
    if (msg.debugArtifacts?.results && files.length > 0) {
      for (let fi = 0; fi < files.length; fi++) {
        const result = msg.debugArtifacts.results.find(
          (r) => r.csv_data && r.status === 'rendered',
        );
        if (result?.csv_data) {
          files[fi].csv_data = result.csv_data;
        }
      }
    }

    return {
      seq: i + 1,
      role: msg.role,
      text: msg.text,
      created_at: now, // precise timestamps not available on ChatEntry; conversation-level is sufficient
      charts,
      files,
      debug_artifacts: msg.debugArtifacts ?? null,
      db_table: 'chat_messages',
      db_fields: ['id', 'conversation_id', 'role', 'content', 'turn_seq', 'created_at'],
    };
  });

  return {
    export_metadata: metadata,
    conversation,
    turns,
  };
}

// ---------------------------------------------------------------------------
// downloadJsonExport — triggers a browser download of the JSON
// ---------------------------------------------------------------------------

export function downloadJsonExport(data: ChatDebugExport, filename?: string): void {
  const json = JSON.stringify(data, null, 2);
  const blob = new Blob([json], { type: 'application/json' });
  const objectUrl = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = objectUrl;
  a.download = filename ?? `livereview-debug-export-${data.conversation.id}.json`;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(objectUrl);
}
