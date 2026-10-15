import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import LanguageDetector from 'i18next-browser-languagedetector';

// Import translation files
// In a real project, translation files are usually placed in public/locales and loaded via http-backend
// or imported directly. Here, we import them directly for demonstration purposes.
import translationEN from './locales/en/translation.json';
import translationZH from './locales/zh/translation.json';

const resources = {
  en: {
    translation: translationEN
  },
  zh: {
    translation: translationZH
  }
};

i18n
  // Detect user language
  // Documentation: https://github.com/i18next/i18next-browser-languageDetector
  .use(LanguageDetector)
  // Pass the i18n instance to react-i18next
  .use(initReactI18next)
  // Initialize i18next
  // All configuration options: https://www.i18next.com/overview/configuration-options
  .init({
    resources,
    fallbackLng: 'en',

    interpolation: {
      escapeValue: false, // React already safes from XSS
    }
  });

// Keep <html lang> in sync with the active UI language (a11y).
const setLang = (lng) => {
  document.documentElement.lang = lng === 'zh' ? 'zh-CN' : 'en';
};
setLang(i18n.language);
i18n.on('languageChanged', setLang);

export default i18n;
