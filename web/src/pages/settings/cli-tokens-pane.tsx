import { App, Button, Popconfirm, Tag } from "antd";
import { KeyRound, Trash2 } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

import { WorkspaceErrorState, WorkspaceState } from "@/components/layout/workspace-state";
import { StatusBadge } from "@/components/ui/base/badges";
import { currentLocale } from "@/i18n";
import { listApiTokens, revokeApiToken, type ApiToken } from "@/services/api/auth";

const SCOPE_LABEL_KEYS: Record<string, string> = {
    read: "account.settings.cliTokens.scopes.read",
    "canvas:write": "account.settings.cliTokens.scopes.canvasWrite",
    "asset:upload": "account.settings.cliTokens.scopes.assetUpload",
    "generate:image": "account.settings.cliTokens.scopes.generateImage",
    "generate:video": "account.settings.cliTokens.scopes.generateVideo",
    "generate:audio": "account.settings.cliTokens.scopes.generateAudio",
    "task:write": "account.settings.cliTokens.scopes.taskWrite",
    "agent:run": "account.settings.cliTokens.scopes.agentRun",
};

function errorText(error: unknown, fallback: string) {
    return error instanceof Error && error.message ? error.message : fallback;
}

function formatTime(value: string | undefined, empty: string) {
    if (!value) return empty;
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? value : date.toLocaleString(currentLocale());
}

function tokenStatus(token: ApiToken): "active" | "revoked" | "expired" {
    if (token.revokedAt) return "revoked";
    if (new Date(token.expiresAt).getTime() <= Date.now()) return "expired";
    return "active";
}

export function CliTokensPane() {
    const { t } = useTranslation();
    const { message } = App.useApp();
    const [tokens, setTokens] = useState<ApiToken[]>([]);
    const [loading, setLoading] = useState(true);
    const [loadError, setLoadError] = useState("");
    const [revokingId, setRevokingId] = useState("");

    const load = useCallback(
        async (signal?: AbortSignal) => {
            setLoading(true);
            setLoadError("");
            try {
                const next = await listApiTokens(signal);
                if (!signal?.aborted) setTokens(next);
            } catch (error) {
                if (!signal?.aborted) setLoadError(errorText(error, t("account.settings.cliTokens.loadFailed")));
            } finally {
                if (!signal?.aborted) setLoading(false);
            }
        },
        [t],
    );

    useEffect(() => {
        const controller = new AbortController();
        void load(controller.signal);
        return () => controller.abort();
    }, [load]);

    const revoke = async (token: ApiToken) => {
        setRevokingId(token.id);
        try {
            await revokeApiToken(token.id);
            message.success(t("account.settings.cliTokens.revoked", { name: token.name }));
            await load();
        } catch (error) {
            message.error(errorText(error, t("account.settings.cliTokens.revokeFailed")));
        } finally {
            setRevokingId("");
        }
    };

    const sorted = [...tokens].sort((left, right) => {
        const leftActive = tokenStatus(left) === "active" ? 1 : 0;
        const rightActive = tokenStatus(right) === "active" ? 1 : 0;
        if (leftActive !== rightActive) return rightActive - leftActive;
        return new Date(right.createdAt).getTime() - new Date(left.createdAt).getTime();
    });

    if (loadError && !tokens.length) {
        return <WorkspaceErrorState compact title={t("account.settings.cliTokens.loadFailed")} description={loadError} onRetry={() => void load()} />;
    }

    return (
        <div className="flex flex-col gap-4">
            <div className="rounded-xl border border-dashed border-border/70 bg-surface-secondary/40 px-3 py-2.5 text-caption leading-5 text-muted-foreground">
                <p>{t("account.settings.cliTokens.hint")}</p>
                <code className="mt-1 inline-block rounded bg-surface-primary px-1.5 py-0.5 text-foreground/80">yingce login</code>
            </div>

            {loading && !tokens.length ? (
                <div className="space-y-2" aria-busy="true" aria-live="polite">
                    {[0, 1, 2].map((index) => (
                        <div key={index} className="h-[112px] animate-pulse rounded-xl bg-surface-secondary" />
                    ))}
                </div>
            ) : null}

            {!loading && !sorted.length ? <WorkspaceState compact icon="empty" className="min-h-[200px] py-10" title={t("account.settings.cliTokens.emptyTitle")} description={t("account.settings.cliTokens.emptyDescription")} /> : null}

            {sorted.length ? (
                <div className="space-y-2">
                    {sorted.map((token) => {
                        const status = tokenStatus(token);
                        const active = status === "active";
                        return (
                            <article key={token.id} className={`rounded-xl border border-border/60 bg-surface-secondary/40 p-3 ${active ? "" : "opacity-70"}`} aria-label={token.name}>
                                <div className="flex flex-wrap items-start gap-2">
                                    <div className="min-w-0 flex-1">
                                        <div className="flex min-w-0 items-center gap-2">
                                            <KeyRound className="size-4 shrink-0 text-muted-foreground" />
                                            <h3 className="truncate text-sm font-medium text-foreground">{token.name}</h3>
                                            <StatusBadge tone={active ? "success" : status === "expired" ? "warning" : "neutral"} label={t(`account.settings.cliTokens.status.${status}`)} size="sm" />
                                        </div>
                                        <p className="mt-1 text-caption text-muted-foreground">
                                            <code className="rounded bg-surface-primary px-1.5 py-0.5">{token.tokenPrefix}…</code>
                                        </p>
                                    </div>
                                    {active ? (
                                        <Popconfirm
                                            title={t("account.settings.cliTokens.revokeTitle")}
                                            description={t("account.settings.cliTokens.revokeConfirm", { name: token.name })}
                                            okText={t("common.delete")}
                                            cancelText={t("common.cancel")}
                                            okButtonProps={{ danger: true, loading: revokingId === token.id }}
                                            onConfirm={() => void revoke(token)}
                                        >
                                            <Button danger size="small" icon={<Trash2 className="size-3.5" />} loading={revokingId === token.id}>
                                                {t("account.settings.cliTokens.revoke")}
                                            </Button>
                                        </Popconfirm>
                                    ) : null}
                                </div>
                                <dl className="mt-3 grid gap-2 text-caption text-muted-foreground sm:grid-cols-3">
                                    <div>
                                        <dt>{t("account.settings.cliTokens.createdAt")}</dt>
                                        <dd className="mt-0.5 text-foreground/75">{formatTime(token.createdAt, "—")}</dd>
                                    </div>
                                    <div>
                                        <dt>{t("account.settings.cliTokens.lastUsedAt")}</dt>
                                        <dd className="mt-0.5 text-foreground/75">{formatTime(token.lastUsedAt, t("account.settings.cliTokens.neverUsed"))}</dd>
                                    </div>
                                    <div>
                                        <dt>{t("account.settings.cliTokens.expiresAt")}</dt>
                                        <dd className="mt-0.5 text-foreground/75">{formatTime(token.expiresAt, "—")}</dd>
                                    </div>
                                </dl>
                                <div className="mt-2 flex flex-wrap gap-1">
                                    {token.scopes.map((scope) => (
                                        <Tag key={scope}>{SCOPE_LABEL_KEYS[scope] ? t(SCOPE_LABEL_KEYS[scope]) : scope}</Tag>
                                    ))}
                                </div>
                            </article>
                        );
                    })}
                </div>
            ) : null}
        </div>
    );
}
