import { useState, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Search } from 'lucide-react'
import { Input } from '@/components/ui/input'
import {
  getAllRegistryModels,
  type ModelRegistryEntry,
} from '@/lib/model-registry'

function formatTokenCount(tokens: number): string {
  if (tokens >= 1_000_000) {
    const val = tokens / 1_000_000
    return val % 1 === 0 ? `${val}M` : `${val.toFixed(1)}M`
  }
  if (tokens >= 1_000) {
    const val = tokens / 1_000
    return val % 1 === 0 ? `${val}K` : `${val.toFixed(1)}K`
  }
  return String(tokens)
}

export function ModelsPane() {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  const allModels = useMemo(() => getAllRegistryModels(), [])

  const filteredModels = useMemo(() => {
    if (!search.trim()) return allModels
    const q = search.trim().toLowerCase()
    return allModels.filter(
      m =>
        m.id.toLowerCase().includes(q) ||
        m.name.toLowerCase().includes(q) ||
        m.aliases.some(a => a.toLowerCase().includes(q)) ||
        m.platform.toLowerCase().includes(q)
    )
  }, [allModels, search])

  return (
    <div className="space-y-6">
      <div>
        <h3 className="text-lg font-medium">{t('preferences.models.title')}</h3>
        <p className="text-sm text-muted-foreground">
          {t('preferences.models.description')}
        </p>
      </div>

      <div className="relative">
        <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
        <Input
          placeholder={t('preferences.models.searchPlaceholder')}
          value={search}
          onChange={e => setSearch(e.target.value)}
          className="pl-8"
        />
      </div>

      <div className="rounded-md border">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b bg-muted/50">
              <th className="px-3 py-2 text-left font-medium">
                {t('preferences.models.colId')}
              </th>
              <th className="px-3 py-2 text-left font-medium">
                {t('preferences.models.colName')}
              </th>
              <th className="px-3 py-2 text-left font-medium">
                {t('preferences.models.colPlatform')}
              </th>
              <th className="px-3 py-2 text-right font-medium">
                {t('preferences.models.colContext')}
              </th>
              <th className="px-3 py-2 text-right font-medium">
                {t('preferences.models.colMaxOutput')}
              </th>
            </tr>
          </thead>
          <tbody>
            {filteredModels.map((model: ModelRegistryEntry) => (
              <tr
                key={model.id}
                className="border-b last:border-0 hover:bg-muted/30"
              >
                <td className="px-3 py-2 font-mono text-xs">
                  {model.id}
                  {model.aliases.length > 0 && (
                    <div className="text-muted-foreground mt-0.5">
                      {model.aliases.join(', ')}
                    </div>
                  )}
                </td>
                <td className="px-3 py-2">{model.name}</td>
                <td className="px-3 py-2">
                  <span className="inline-flex items-center rounded-full border px-2 py-0.5 text-xs">
                    {model.platform}
                  </span>
                </td>
                <td className="px-3 py-2 text-right font-mono text-xs">
                  {formatTokenCount(model.contextWindow)}
                </td>
                <td className="px-3 py-2 text-right font-mono text-xs">
                  {model.maxOutputTokens
                    ? formatTokenCount(model.maxOutputTokens)
                    : '-'}
                </td>
              </tr>
            ))}
            {filteredModels.length === 0 && (
              <tr>
                <td
                  colSpan={5}
                  className="px-3 py-6 text-center text-muted-foreground"
                >
                  {t('preferences.models.noResults')}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      <p className="text-xs text-muted-foreground">
        {t('preferences.models.totalCount', { count: allModels.length })}
      </p>
    </div>
  )
}
