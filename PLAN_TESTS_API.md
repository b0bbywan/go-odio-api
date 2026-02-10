# Plan: Tests Complets pour l'API - Handlers et Error Mapping

## ⚠️ PROBLÈME CRITIQUE IDENTIFIÉ

**`api/services.go` ne gère PAS correctement les erreurs systemd !**

```go
// ❌ ACTUEL (ligne 27-28)
if err := fn(unit, scope); err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)  // Tous → 500
    return
}
```

**Conséquence :**
- `PermissionSystemError` (tentative d'action sur system scope) → 500 au lieu de 403
- `PermissionUserError` (tentative d'action sur user unit non-whitelisté) → 500 au lieu de 403
- **Violation du modèle de sécurité documenté dans le README**

**Solution requise :** Créer `handleSystemdError` comme les autres backends (MPRIS, Bluetooth)

---

## État Actuel de l'API

### ✅ Handlers avec Error Mapping Correct

**1. MPRIS** (`api/players.go`)
- ✅ `handleMPRISError` implémenté
- Mapping: PlayerNotFound→404, InvalidBusName→400, Validation→400, Capability→403, autres→500
- Handlers: ListPlayers, Play, Pause, PlayPause, Stop, Next, Previous, Seek, SetPosition, SetVolume, SetLoop, SetShuffle

**2. Bluetooth** (`api/bluetooth.go`)
- ✅ `handleBluetoothError` implémenté (simple)
- Mapping: succès→202, erreur→500
- Wrapper: `withBluetoothAction`

### ⚠️ Handlers SANS Error Mapping Correct

**3. Systemd** (`api/services.go`) - **PRIORITÉ 1 À FIXER**
- ❌ PAS de `handleSystemdError`
- Handlers: EnableService, DisableService, StartService, StopService, RestartService, ListServices
- Middleware: `withService` (extraction scope + unit)
- **Mapping attendu:**
  - `PermissionSystemError` → 403 Forbidden
  - `PermissionUserError` → 403 Forbidden
  - Autres erreurs → 500 Internal Server Error

**4. PulseAudio** (`api/audio.go`)
- ❌ PAS de `handlePulseAudioError` (gestion inline)
- Handlers: MuteClient, MuteMaster, SetVolumeClient, SetVolumeMaster
- Middleware: `withSink` (extraction sink ID)
- Actuellement tous les cas → 500 (acceptable pour l'instant, pas d'erreurs typées dans le backend)

### 📦 Middleware Existants

- ✅ `JSONHandler` - Parse réponse JSON générique
- ✅ `withBody[T]` - Parse body JSON avec validation optionnelle
- ✅ `withPlayer` - Extraction busName MPRIS
- ✅ `withSink` - Extraction sink ID PulseAudio
- ✅ `withService` - Extraction scope + unit systemd (mais pas d'error handling !)
- ✅ `withBluetoothAction` - Wrapper action Bluetooth avec error handling
- ✅ `validateVolume` - Validator pour volume 0.0-1.0

---

## Plan d'Implémentation

### PHASE 0: FIX CRITIQUE - handleSystemdError (AVANT LES TESTS)

**Fichier:** `api/services.go`

**Action:**
1. Créer `handleSystemdError` qui mappe correctement les erreurs
2. Mettre à jour `withService` pour l'utiliser

```go
func handleSystemdError(w http.ResponseWriter, err error) {
    if err == nil {
        w.WriteHeader(http.StatusAccepted)
        return
    }

    // PermissionSystemError → 403 Forbidden
    var permSysErr *systemd.PermissionSystemError
    if errors.As(err, &permSysErr) {
        http.Error(w, err.Error(), http.StatusForbidden)
        return
    }

    // PermissionUserError → 403 Forbidden
    var permUserErr *systemd.PermissionUserError
    if errors.As(err, &permUserErr) {
        http.Error(w, err.Error(), http.StatusForbidden)
        return
    }

    // Autres erreurs → 500
    http.Error(w, err.Error(), http.StatusInternalServerError)
}

func withService(
    sd *systemd.SystemdBackend,
    fn func(string, systemd.UnitScope) error,
) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        scope, ok := systemd.ParseUnitScope(r.PathValue("scope"))
        if !ok {
            http.Error(w, "invalid scope", http.StatusNotFound)
            return
        }

        unit := r.PathValue("unit")
        if unit == "" {
            http.Error(w, "missing unit name", http.StatusNotFound)
            return
        }

        handleSystemdError(w, fn(unit, scope))  // ← Utiliser le handler d'erreur
    }
}
```

**Tests critiques à ajouter:**
- ✅ POST sur system scope → **TOUJOURS 403** (PermissionSystemError)
- ✅ POST sur user scope avec unit non-whitelisté → 403 (PermissionUserError)
- ✅ POST sur user scope avec unit whitelisté → 202 Accepted

---

### PHASE 1: Tests Handlers Systemd (`api/handlers_systemd_test.go`)

**Priorité:** CRITIQUE (après fix Phase 0)

**Tests:**

1. **`TestHandleSystemdError`** - Mapping erreur → HTTP status (**LE PLUS IMPORTANT**)
   - `PermissionSystemError` → 403 Forbidden
   - `PermissionUserError` → 403 Forbidden
   - Erreur générique → 500 Internal Server Error
   - Pas d'erreur → 202 Accepted

2. **`TestWithService`** - Middleware extraction + validation
   - Scope system valide → extrait correctement
   - Scope user valide → extrait correctement
   - Scope invalide → 404 "invalid scope"
   - Unit manquant → 404 "missing unit name"

3. **`TestEnableServiceHandler`** - Enable service
   - Scope **system** → **403 Forbidden** (PermissionSystemError)
   - Scope user + unit whitelisté → 202 Accepted
   - Scope user + unit NON whitelisté → 403 Forbidden (PermissionUserError)
   - Backend error → 500

4. **`TestDisableServiceHandler`**, **`TestStartServiceHandler`**, **`TestStopServiceHandler`**, **`TestRestartServiceHandler`**
   - Même structure que Enable
   - **CRITIQUE:** Toutes les actions sur system scope doivent retourner 403

**Pattern de mock:**
```go
type mockSystemdBackend struct {
    enableFunc  func(string, systemd.UnitScope) error
    disableFunc func(string, systemd.UnitScope) error
    startFunc   func(string, systemd.UnitScope) error
    stopFunc    func(string, systemd.UnitScope) error
    restartFunc func(string, systemd.UnitScope) error
}
```

**Estimé:** ~400-500 lignes, 7 fonctions de test, ~30-40 subtests

---

### PHASE 2: Tests Handlers MPRIS (`api/handlers_mpris_test.go`)

**Priorité:** Haute (API la plus utilisée)

**Tests:**

1. **`TestHandleMPRISError`** - Mapping erreur → HTTP status
   - `PlayerNotFoundError` → 404 Not Found
   - `InvalidBusNameError` → 400 Bad Request
   - `ValidationError` → 400 Bad Request
   - `CapabilityError` → 403 Forbidden
   - Erreur générique → 500 Internal Server Error
   - Pas d'erreur → 202 Accepted

2. **`TestListPlayersHandler`**
   - Succès avec players → array JSON
   - Liste vide → array JSON vide
   - Erreur backend → 500

3. **`TestPlayHandler`, `TestPauseHandler`, `TestPlayPauseHandler`, `TestStopHandler`**
   - Succès → 202 Accepted
   - Player non trouvé → 404
   - Capability error → 403 Forbidden
   - BusName invalide → 400

4. **`TestNextHandler`, `TestPreviousHandler`**
   - Même pattern que play/pause

5. **`TestSeekHandler`**
   - Offset positif valide → 202
   - Offset négatif valide → 202
   - JSON invalide → 400
   - Player non trouvé → 404

6. **`TestSetPositionHandler`**
   - trackID + position valides → 202
   - JSON invalide → 400
   - Validation error → 400

7. **`TestSetVolumeHandler`**
   - Volume valide (0.0-1.0) → 202
   - Volume > 1.0 → 400 (validation error)
   - Volume < 0.0 → 400 (validation error)
   - JSON invalide → 400

8. **`TestSetLoopHandler`, `TestSetShuffleHandler`**
   - Succès → 202
   - Erreurs → codes appropriés

**Estimé:** ~600-800 lignes, 12 fonctions de test, ~40-50 subtests

---

### PHASE 3: Tests Handlers PulseAudio (`api/handlers_audio_test.go`)

**Priorité:** Moyenne

**Tests:**

1. **`TestMuteClientHandler`**
   - Succès → 202
   - Sink manquant → 404 "missing sink"
   - Erreur backend → 500

2. **`TestMuteMasterHandler`**
   - Succès → 202
   - Erreur backend → 500

3. **`TestSetVolumeClientHandler`**
   - Volume valide → 202
   - Volume hors plage → 400 (validation)
   - Sink manquant → 404
   - JSON invalide → 400

4. **`TestSetVolumeMasterHandler`**
   - Même structure que volume client

5. **`TestWithSink`**
   - Sink présent → extrait correctement
   - Sink manquant → 404 "missing sink"

**Estimé:** ~300-400 lignes, 5 fonctions de test, ~15-20 subtests

---

### PHASE 4: Tests Handlers Bluetooth (`api/handlers_bluetooth_test.go`)

**Priorité:** Basse (handlers simples)

**Tests:**

1. **`TestHandleBluetoothError`**
   - Succès → 202
   - Erreur → 500

2. **`TestWithBluetoothAction`**
   - Action réussit → 202
   - Action échoue → 500

**Estimé:** ~100-150 lignes, 2 fonctions de test, ~5-10 subtests

---

### PHASE 5: Tests Middleware (`api/middleware_test.go`)

**Priorité:** Moyenne

**Tests:**

1. **`TestWithBody`**
   - JSON valide → parse réussi
   - JSON invalide → 400 Bad Request
   - Body vide → 400 Bad Request
   - Validation réussit → succès
   - Validation échoue → 400 Bad Request
   - Validator nil → pas de validation

2. **`TestWithPlayer`**
   - BusName valide → extrait correctement
   - Player vide → chaîne vide passée

3. **`TestValidateVolume`**
   - Volumes valides (0.0, 0.5, 1.0) → OK
   - Volumes invalides (-0.1, 1.1) → erreur

**Estimé:** ~200-300 lignes, 3 fonctions de test, ~15-20 subtests

---

## Pattern de Structure de Test

Tous les tests suivent le **pattern table-driven** établi:

```go
func TestHandlerName(t *testing.T) {
    tests := []struct {
        name           string
        setupMock      func() *mockBackend
        requestBody    string
        pathParams     map[string]string
        wantStatusCode int
        wantErrorMatch string
    }{
        {
            name: "success case",
            setupMock: func() *mockBackend {
                return &mockBackend{
                    methodFunc: func(string, systemd.UnitScope) error { return nil },
                }
            },
            pathParams:     map[string]string{"scope": "user", "unit": "test.service"},
            wantStatusCode: http.StatusAccepted,
        },
        {
            name: "system scope always forbidden",
            setupMock: func() *mockBackend {
                return &mockBackend{
                    methodFunc: func(string, systemd.UnitScope) error {
                        return &systemd.PermissionSystemError{Unit: "test.service"}
                    },
                }
            },
            pathParams:     map[string]string{"scope": "system", "unit": "test.service"},
            wantStatusCode: http.StatusForbidden,
            wantErrorMatch: "can not act on system units",
        },
        // ... more test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            mock := tt.setupMock()
            handler := HandlerName(mock)

            req := httptest.NewRequest("POST", "/path", strings.NewReader(tt.requestBody))
            for k, v := range tt.pathParams {
                req.SetPathValue(k, v)
            }
            w := httptest.NewRecorder()

            handler(w, req)

            if w.Code != tt.wantStatusCode {
                t.Errorf("status = %d, want %d", w.Code, tt.wantStatusCode)
            }

            if tt.wantErrorMatch != "" && !strings.Contains(w.Body.String(), tt.wantErrorMatch) {
                t.Errorf("response = %q, want to contain %q", w.Body.String(), tt.wantErrorMatch)
            }
        })
    }
}
```

---

## Fichiers à Créer/Modifier

**Phase 0 (FIX CRITIQUE):**
- ✏️ Modifier: `/home/user/go-odio-api/api/services.go` - Ajouter `handleSystemdError`

**Phases 1-5 (TESTS):**
- ✨ Créer: `/home/user/go-odio-api/api/handlers_systemd_test.go`
- ✨ Créer: `/home/user/go-odio-api/api/handlers_mpris_test.go`
- ✨ Créer: `/home/user/go-odio-api/api/handlers_audio_test.go`
- ✨ Créer: `/home/user/go-odio-api/api/handlers_bluetooth_test.go`
- ✨ Créer: `/home/user/go-odio-api/api/middleware_test.go`

**Fichiers de référence:**
- `/home/user/go-odio-api/api/json_test.go` - Pattern et style de test existants
- `/home/user/go-odio-api/api/players.go` - Référence pour `handleMPRISError`
- `/home/user/go-odio-api/backend/systemd/systemd.go` - Logique de sécurité systemd
- `/home/user/go-odio-api/backend/systemd/types.go` - Types d'erreurs systemd

---

## Vérification

Après chaque phase:

```bash
# Tests du fichier spécifique
go test ./api/handlers_systemd_test.go -v

# Tous les tests API
go test ./api/... -v

# Couverture
go test -cover ./api/...

# Vérifier que les erreurs sont bien 403 pour system scope
go test ./api/... -v -run ".*System.*"
```

**Critères de succès:**
- ✅ Tous les tests passent
- ✅ **POST sur system scope retourne TOUJOURS 403** (PermissionSystemError)
- ✅ POST sur user scope non-whitelisté retourne 403 (PermissionUserError)
- ✅ Mapping d'erreurs vérifié pour tous les backends
- ✅ Couverture >85% pour le package API
- ✅ Pas de panics au démarrage

---

## Scope Total

- **1 fichier à modifier** (fix critique)
- **5 nouveaux fichiers de test**
- **29 fonctions de test**
- **120-140 subtests**
- **~1700-2200 lignes de code de test**
- **Couverture cible: >85%**

---

## Notes Importantes

1. **SÉCURITÉ CRITIQUE:** Tous les tests doivent vérifier que les actions sur `system` scope retournent **403 Forbidden**, jamais 202 ou 500.

2. **Pattern d'erreur:** Utiliser `errors.As()` pour type assertion comme dans `handleMPRISError`

3. **Mocks simples:** Pas de lib externe, juste des structs avec fonctions callback

4. **Documentation:** Tous les tests doivent documenter le comportement de sécurité attendu
