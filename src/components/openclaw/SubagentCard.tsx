import { useTranslation } from 'react-i18next'
import { Pencil, Trash2, Bot } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Switch } from '@/components/ui/switch'
import { type OpenClawSubAgent } from '@/lib/bindings'

interface SubagentCardProps {
  agent: OpenClawSubAgent
  isAllowed: boolean
  onToggleAllow: () => void
  onEdit: () => void
  onDelete: () => void
}

export function SubagentCard({
  agent,
  isAllowed,
  onToggleAllow,
  onEdit,
  onDelete,
}: SubagentCardProps) {
  const { t } = useTranslation()

  return (
    <div className="flex items-center gap-3 p-3 border rounded-lg hover:bg-muted/30 transition-colors">
      {/* Allow Toggle */}
      <Switch
        checked={isAllowed}
        onCheckedChange={onToggleAllow}
        className="shrink-0"
      />

      {/* Emoji / Icon */}
      <div className="text-2xl w-10 h-10 flex items-center justify-center shrink-0 rounded-md bg-muted">
        {agent.identity?.emoji ?? (
          <Bot className="h-5 w-5 text-muted-foreground" />
        )}
      </div>

      {/* Info */}
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2">
          <span className="font-medium text-sm truncate">
            {agent.identity?.name ?? agent.name ?? agent.id}
          </span>
          <Badge variant="outline" className="text-xs shrink-0">
            {agent.id}
          </Badge>
        </div>
        <div className="flex items-center gap-2 mt-1">
          {agent.model?.primary && (
            <span className="text-xs text-muted-foreground font-mono truncate">
              {agent.model.primary}
            </span>
          )}
          {!agent.model?.primary && (
            <span className="text-xs text-muted-foreground italic">
              {t('openclaw.subagents.noModel')}
            </span>
          )}
          {agent.tools?.profile && (
            <Badge variant="secondary" className="text-xs shrink-0">
              {agent.tools.profile}
            </Badge>
          )}
        </div>
      </div>

      {/* Actions */}
      <div className="flex items-center gap-1 shrink-0">
        <Button
          variant="ghost"
          size="icon"
          className="h-8 w-8"
          onClick={onEdit}
          title={t('common.edit')}
        >
          <Pencil className="h-4 w-4" />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className="h-8 w-8"
          onClick={onDelete}
          title={t('common.delete')}
        >
          <Trash2 className="h-4 w-4" />
        </Button>
      </div>
    </div>
  )
}
