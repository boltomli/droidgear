use super::*;

pub(super) fn handle_omp_key(app: &mut app::App, code: KeyCode) -> Option<Action> {
    match code {
        KeyCode::Esc | KeyCode::Char('q') => app.go_back(),
        KeyCode::Down => app.omp_index = app.omp_index.saturating_add(1),
        KeyCode::Up => app.omp_index = app.omp_index.saturating_sub(1),
        KeyCode::Char('r') => refresh_omp(app),
        KeyCode::Char('n') => {
            app.modal = Some(app::Modal::Input {
                title: "New OMP profile name".to_string(),
                value: String::new(),
                cursor: usize::MAX,
                is_secret: false,
                action: app::InputAction::OmpCreateProfile,
            });
        }
        KeyCode::Enter | KeyCode::Char('e') => {
            if let Some(p) = app.omp_profiles.get(app.omp_index) {
                app.omp_detail_id = Some(p.id.clone());
                app.omp_detail_field_index = 0;
                app.omp_provider_index = 0;
                app.omp_provider_field_index = 0;
                app.omp_model_index = 0;
                app.omp_model_field_index = 0;
                app.screen = app::Screen::OmpProfile;
                refresh_omp_detail(app);
            }
        }
        KeyCode::Char('a') => {
            if let Some(p) = app.omp_profiles.get(app.omp_index) {
                app.modal = Some(app::Modal::Confirm {
                    message: format!("Apply OMP profile '{}'?", p.name),
                    action: app::ConfirmAction::OmpApply { id: p.id.clone() },
                });
            }
        }
        KeyCode::Char('d') => {
            if let Some(p) = app.omp_profiles.get(app.omp_index) {
                app.modal = Some(app::Modal::Confirm {
                    message: format!("Delete OMP profile '{}'?", p.name),
                    action: app::ConfirmAction::OmpDelete { id: p.id.clone() },
                });
            }
        }
        KeyCode::Char('c') => {
            if let Some(p) = app.omp_profiles.get(app.omp_index) {
                app.modal = Some(app::Modal::Input {
                    title: "Duplicate profile name".to_string(),
                    value: format!("{} (copy)", p.name),
                    cursor: usize::MAX,
                    is_secret: false,
                    action: app::InputAction::OmpDuplicate { id: p.id.clone() },
                });
            }
        }
        _ => {}
    }
    None
}

pub(super) fn handle_omp_profile_key(app: &mut app::App, code: KeyCode) -> Option<Action> {
    let Some(profile_id) = app.omp_detail_id.clone() else {
        app.screen = app::Screen::Omp;
        return None;
    };
    let Some(profile) = app.omp_detail.as_ref() else {
        return None;
    };

    let fields_count = 2usize; // Name, Description
    let provider_count = profile.providers.len();

    match code {
        KeyCode::Esc | KeyCode::Char('q') => {
            app.go_back();
        }
        KeyCode::Down => {
            let total = fields_count + provider_count;
            if total > 0 {
                app.omp_detail_field_index = (app.omp_detail_field_index + 1).min(total - 1);
            }
        }
        KeyCode::Up => {
            app.omp_detail_field_index = app.omp_detail_field_index.saturating_sub(1);
        }
        KeyCode::Char('r') => refresh_omp_detail(app),
        KeyCode::Char('a') => {
            app.modal = Some(app::Modal::Confirm {
                message: format!("Apply OMP profile '{}'?", profile.name),
                action: app::ConfirmAction::OmpApply {
                    id: profile_id.clone(),
                },
            });
        }
        KeyCode::Char('p') if provider_count > 0 => {
            // Navigate to provider detail for the currently selected provider
            app.omp_provider_index = app.omp_detail_field_index.saturating_sub(fields_count);
            app.omp_provider_field_index = 0;
            app.omp_model_index = 0;
            app.omp_model_field_index = 0;
            app.screen = app::Screen::OmpProvider;
        }
        KeyCode::Char('l') => {
            if let Err(e) = omp_load_from_live_config(app, &profile_id) {
                app.set_toast(e.to_string(), true);
            } else {
                app.set_toast("Loaded from live config", false);
                refresh_omp_detail(app);
            }
        }
        KeyCode::Enter | KeyCode::Char('e') => {
            if app.omp_detail_field_index < fields_count {
                // Editing profile fields
                match app.omp_detail_field_index {
                    0 => {
                        app.modal = Some(app::Modal::Input {
                            title: "Profile name".to_string(),
                            value: profile.name.clone(),
                            cursor: usize::MAX,
                            is_secret: false,
                            action: app::InputAction::OmpSetProfileName {
                                id: profile_id.clone(),
                            },
                        });
                    }
                    1 => {
                        app.modal = Some(app::Modal::Input {
                            title: "Profile description".to_string(),
                            value: profile.description.clone().unwrap_or_default(),
                            cursor: usize::MAX,
                            is_secret: false,
                            action: app::InputAction::OmpSetProfileDescription {
                                id: profile_id.clone(),
                            },
                        });
                    }
                    _ => {}
                }
            } else {
                // Open the selected provider's detail screen
                let prov_idx = app.omp_detail_field_index - fields_count;
                app.omp_provider_index = prov_idx;
                app.omp_provider_field_index = 0;
                app.omp_model_index = 0;
                app.omp_model_field_index = 0;
                app.screen = app::Screen::OmpProvider;
            }
        }
        KeyCode::Char('n') => {
            app.modal = Some(app::Modal::Input {
                title: "New provider id".to_string(),
                value: String::new(),
                cursor: usize::MAX,
                is_secret: false,
                action: app::InputAction::OmpAddProvider {
                    profile_id: profile_id.clone(),
                },
            });
        }
        KeyCode::Char('i') => {
            refresh_channels(app);
            let options: Vec<String> = app
                .channels
                .iter()
                .filter(|c| c.enabled)
                .map(|c| format!("{} ({})", c.name, c.base_url))
                .collect();
            if options.is_empty() {
                app.set_toast("No channels configured", true);
            } else {
                app.modal = Some(app::Modal::Input {
                    title: "New provider id (create from channel)".to_string(),
                    value: String::new(),
                    cursor: usize::MAX,
                    is_secret: false,
                    action: app::InputAction::OmpAddProviderFromChannel {
                        profile_id: profile_id.clone(),
                    },
                });
            }
        }
        KeyCode::Char('d') if app.omp_detail_field_index >= fields_count => {
            let prov_idx = app.omp_detail_field_index - fields_count;
            // Get the provider ID at this index
            let mut keys: Vec<String> = profile.providers.keys().cloned().collect();
            keys.sort_by_key(|a| a.to_lowercase());
            if let Some(provider_id) = keys.get(prov_idx).cloned() {
                app.modal = Some(app::Modal::Confirm {
                    message: format!("Delete provider '{provider_id}'?"),
                    action: app::ConfirmAction::OmpDeleteProvider {
                        profile_id: profile_id.clone(),
                        provider_id,
                    },
                });
            }
        }
        _ => {}
    }

    None
}

pub(super) fn handle_omp_provider_key(app: &mut app::App, code: KeyCode) -> Option<Action> {
    let Some(profile_id) = app.omp_detail_id.clone() else {
        app.screen = app::Screen::Omp;
        return None;
    };
    let Some(provider_id) = app.omp_current_provider_id() else {
        app.screen = app::Screen::OmpProfile;
        return None;
    };
    let Some(profile) = app.omp_detail.as_ref() else {
        return None;
    };
    let Some(config) = profile.providers.get(&provider_id) else {
        app.set_toast("Provider not found", true);
        app.screen = app::Screen::OmpProfile;
        return None;
    };

    match code {
        KeyCode::Esc | KeyCode::Char('q') => app.go_back(),
        KeyCode::Down => {
            app.omp_provider_field_index = app.omp_provider_field_index.saturating_add(1)
        }
        KeyCode::Up => {
            app.omp_provider_field_index = app.omp_provider_field_index.saturating_sub(1)
        }
        KeyCode::Char('m') => {
            app.omp_model_index = 0;
            app.omp_model_field_index = 0;
            app.screen = app::Screen::OmpModel;
        }
        KeyCode::Enter | KeyCode::Char('e') => match app.omp_provider_field_index {
            0 => {
                app.modal = Some(app::Modal::Input {
                    title: "Base URL".to_string(),
                    value: config.base_url.clone().unwrap_or_default(),
                    cursor: usize::MAX,
                    is_secret: false,
                    action: app::InputAction::OmpSetProviderBaseUrl {
                        profile_id: profile_id.clone(),
                        provider_id: provider_id.clone(),
                    },
                });
            }
            1 => {
                let options = vec![
                    "openai-completions".to_string(),
                    "openai-responses".to_string(),
                    "anthropic-messages".to_string(),
                    "google-generative-ai".to_string(),
                ];
                let index = config
                    .api
                    .as_deref()
                    .and_then(|v| options.iter().position(|o| o == v))
                    .unwrap_or(0);
                app.modal = Some(app::Modal::Select {
                    title: "API type".to_string(),
                    options,
                    index,
                    action: app::SelectAction::OmpSetProviderApi {
                        profile_id: profile_id.clone(),
                        provider_id: provider_id.clone(),
                    },
                });
            }
            2 => {
                app.modal = Some(app::Modal::Input {
                    title: "API key".to_string(),
                    value: config.api_key.clone().unwrap_or_default(),
                    cursor: usize::MAX,
                    is_secret: true,
                    action: app::InputAction::OmpSetProviderApiKey {
                        profile_id: profile_id.clone(),
                        provider_id: provider_id.clone(),
                    },
                });
            }
            _ => {}
        },
        KeyCode::Char('n') => {
            app.modal = Some(app::Modal::Input {
                title: "New model id".to_string(),
                value: String::new(),
                cursor: usize::MAX,
                is_secret: false,
                action: app::InputAction::OmpAddModel {
                    profile_id: profile_id.clone(),
                    provider_id: provider_id.clone(),
                },
            });
        }
        KeyCode::Char('d')
            if !config.models.is_empty() && app.omp_model_index < config.models.len() =>
        {
            let model_id = config.models[app.omp_model_index].id.clone();
            app.modal = Some(app::Modal::Confirm {
                message: format!("Delete model '{model_id}'?"),
                action: app::ConfirmAction::OmpDeleteModel {
                    profile_id: profile_id.clone(),
                    provider_id: provider_id.clone(),
                    model_index: app.omp_model_index,
                },
            });
        }
        KeyCode::Char('i') => {
            // Import from channel: refresh channels first
            refresh_channels(app);
            let options: Vec<String> = app
                .channels
                .iter()
                .filter(|c| c.enabled)
                .map(|c| format!("{} ({})", c.name, c.base_url))
                .collect();
            if options.is_empty() {
                app.set_toast("No channels configured", true);
            } else {
                app.modal = Some(app::Modal::Select {
                    title: "Import from channel".to_string(),
                    options,
                    index: 0,
                    action: app::SelectAction::OmpImportFromChannel {
                        profile_id: profile_id.clone(),
                        provider_id: provider_id.clone(),
                    },
                });
            }
        }
        _ => {}
    }

    None
}

pub(super) fn handle_omp_model_key(app: &mut app::App, code: KeyCode) -> Option<Action> {
    let Some(profile_id) = app.omp_detail_id.clone() else {
        app.screen = app::Screen::Omp;
        return None;
    };
    let Some(provider_id) = app.omp_current_provider_id() else {
        app.screen = app::Screen::OmpProfile;
        return None;
    };
    let Some(profile) = app.omp_detail.as_ref() else {
        return None;
    };
    let Some(provider) = profile.providers.get(&provider_id) else {
        app.set_toast("Provider not found", true);
        app.screen = app::Screen::OmpProfile;
        return None;
    };
    let model_index = app.omp_model_index;
    let Some(model) = provider.models.get(model_index) else {
        app.set_toast("Model not found", true);
        app.screen = app::Screen::OmpProvider;
        return None;
    };

    match code {
        KeyCode::Esc | KeyCode::Char('q') => app.go_back(),
        KeyCode::Down => app.omp_model_field_index = app.omp_model_field_index.saturating_add(1),
        KeyCode::Up => app.omp_model_field_index = app.omp_model_field_index.saturating_sub(1),
        KeyCode::Enter | KeyCode::Char('e') => match app.omp_model_field_index {
            0 => {
                app.modal = Some(app::Modal::Input {
                    title: "Model id".to_string(),
                    value: model.id.clone(),
                    cursor: usize::MAX,
                    is_secret: false,
                    action: app::InputAction::OmpSetModelId {
                        profile_id: profile_id.clone(),
                        provider_id: provider_id.clone(),
                        model_index,
                    },
                });
            }
            1 => {
                app.modal = Some(app::Modal::Input {
                    title: "Model name".to_string(),
                    value: model.name.clone().unwrap_or_default(),
                    cursor: usize::MAX,
                    is_secret: false,
                    action: app::InputAction::OmpSetModelName {
                        profile_id: profile_id.clone(),
                        provider_id: provider_id.clone(),
                        model_index,
                    },
                });
            }
            2 => {
                // Toggle reasoning
                if let Err(e) =
                    omp_toggle_model_reasoning(app, &profile_id, &provider_id, model_index)
                {
                    app.set_toast(e.to_string(), true);
                } else {
                    refresh_omp_detail(app);
                }
            }
            3 => {
                // Toggle input type (text <-> text+image)
                if let Err(e) = omp_toggle_model_input(app, &profile_id, &provider_id, model_index)
                {
                    app.set_toast(e.to_string(), true);
                } else {
                    refresh_omp_detail(app);
                }
            }
            4 => {
                app.modal = Some(app::Modal::Input {
                    title: "Context window".to_string(),
                    value: model.context_window.to_string(),
                    cursor: usize::MAX,
                    is_secret: false,
                    action: app::InputAction::OmpSetModelContextWindow {
                        profile_id: profile_id.clone(),
                        provider_id: provider_id.clone(),
                        model_index,
                    },
                });
            }
            5 => {
                app.modal = Some(app::Modal::Input {
                    title: "Max tokens".to_string(),
                    value: model.max_tokens.to_string(),
                    cursor: usize::MAX,
                    is_secret: false,
                    action: app::InputAction::OmpSetModelMaxTokens {
                        profile_id: profile_id.clone(),
                        provider_id: provider_id.clone(),
                        model_index,
                    },
                });
            }
            6 => {
                // Edit cost as comma-separated: input,output,cacheRead,cacheWrite
                let cost_str = match &model.cost {
                    Some(c) => format!(
                        "{},{},{},{}",
                        c.input, c.output, c.cache_read, c.cache_write
                    ),
                    None => "0,0,0,0".to_string(),
                };
                app.modal = Some(app::Modal::Input {
                    title: "Cost (input,output,cacheRead,cacheWrite)".to_string(),
                    value: cost_str,
                    cursor: usize::MAX,
                    is_secret: false,
                    action: app::InputAction::OmpSetModelCost {
                        profile_id: profile_id.clone(),
                        provider_id: provider_id.clone(),
                        model_index,
                    },
                });
            }
            _ => {}
        },
        _ => {}
    }

    None
}

pub(super) fn omp_toggle_model_reasoning(
    app: &mut app::App,
    profile_id: &str,
    provider_id: &str,
    model_index: usize,
) -> anyhow::Result<()> {
    let mut profile = droidgear_core::omp::get_omp_profile_for_home(&app.home_dir, profile_id)
        .map_err(anyhow::Error::msg)?;
    let Some(provider) = profile.providers.get_mut(provider_id) else {
        return Err(anyhow::Error::msg("Provider not found"));
    };
    let Some(model) = provider.models.get_mut(model_index) else {
        return Err(anyhow::Error::msg("Model not found"));
    };
    model.reasoning = !model.reasoning;
    droidgear_core::omp::save_omp_profile_for_home(&app.home_dir, profile)
        .map_err(anyhow::Error::msg)?;
    Ok(())
}

pub(super) fn omp_toggle_model_input(
    app: &mut app::App,
    profile_id: &str,
    provider_id: &str,
    model_index: usize,
) -> anyhow::Result<()> {
    let mut profile = droidgear_core::omp::get_omp_profile_for_home(&app.home_dir, profile_id)
        .map_err(anyhow::Error::msg)?;
    let Some(provider) = profile.providers.get_mut(provider_id) else {
        return Err(anyhow::Error::msg("Provider not found"));
    };
    let Some(model) = provider.models.get_mut(model_index) else {
        return Err(anyhow::Error::msg("Model not found"));
    };
    // Toggle between ["text"] and ["text", "image"]
    if model.input.iter().any(|t| t == "image") {
        model.input.retain(|t| t != "image");
    } else {
        model.input.push("image".to_string());
        model.input.sort();
        model.input.dedup();
    }
    droidgear_core::omp::save_omp_profile_for_home(&app.home_dir, profile)
        .map_err(anyhow::Error::msg)?;
    Ok(())
}
