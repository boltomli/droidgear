import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import en from '../../locales/en.json'
import zh from '../../locales/zh.json'

const resources = {
  en: { translation: en },
  zh: { translation: zh },
}

i18n.use(initReactI18next).init({
  resources,
  lng: 'en',
  fallbackLng: 'en',
  interpolation: {
    escapeValue: false, // React already escapes
  },
})

// Update document lang on language change
i18n.on('languageChanged', lng => {
  document.documentElement.lang = lng
})

export default i18n

// Export for use in non-React contexts (like menu building)
export { i18n }

// Helper to get available languages
export const availableLanguages = Object.keys(resources)
