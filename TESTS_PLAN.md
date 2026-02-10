# Plan: Tests Complets pour l'API - Middleware et Handlers

## Contexte

Le handler UI a récemment causé des panics au démarrage à cause de tests manquants sur les templates. Pour éviter des problèmes similaires avec la couche API, nous avons besoin de tests complets pour tous les middleware et handlers.

**État actuel:**
- Seulement `api/json_test.go` existe avec tests basiques pour `JSONHandler`
- Aucun test pour les middleware critiques: `withBody`, `withPlayer`, `withSink`, `withService`, `withBluetoothAction`
- Aucun test pour les handlers d'endpoints (players, audio, services, bluetooth)
- Aucun test de mapping erreur → code HTTP
- Risque de panics en production ou réponses d'erreur incorrectes

**Objectif:** Ajouter une couverture de tests complète en suivant les patterns établis du codebase (table-driven tests, pas de lib de mocking externe, `httptest` pour tests HTTP).

---

## Plan d'Implémentation

### Phase 1: Tests Middleware (`api/middleware_test.go`) - Priorité 1

**Fichier:** `/home/user/go-odio-api/api/middleware_test.go`

Tests fondamentaux dont dépendent les autres handlers. Tests complets pour:

1. **`TestWithBody`** - Middleware de parsing JSON body générique
   - Payload JSON valide → parse réussi
   - JSON invalide/malformé → 400 Bad Request
   - Body vide/EOF → 400 Bad Request
   - Validation custom réussit → succès
   - Validation custom échoue → 400 Bad Request
   - Validator nil → pas de validation

2. **`TestWithPlayer`** - Extraction paramètre path player
   - BusName valide (ex: `org.mpris.MediaPlayer2.spotify`) → extrait correctement
   - Player vide → chaîne vide passée au next handler
   - Caractères spéciaux (points, underscores) → gérés correctement

3. **`TestWithSink`** - Extraction paramètre path sink
   - Sink valide → extrait correctement
   - Sink vide → 404 avec message "missing sink"
   - ID sink numérique → géré correctement

4. **`TestWithService`** - Extraction scope et unit
   - Scope system + unit → parsé correctement
   - Scope user + unit → parsé correctement
   - Scope invalide → 404 "invalid scope"
   - Unit manquant/vide → 404 "missing unit name"

5. **`TestWithBluetoothAction`** - Wrapper action Bluetooth
   - Action réussit → 202 Accepted
   - Action échoue → 500 avec message d'erreur

6. **`TestValidateVolume`** - Helper validation volume
   - Volumes valides (0.0, 0.5, 1.0) → passent
   - Volumes invalides (-0.1, 1.1) → échouent avec erreur

**Pattern de mock:** Mocks simples basés sur fonctions sans lib externe:
```go
type mockPulseBackend struct {
    muteFunc func(string) error
}
```

**Estimé:** ~300-400 lignes, 6 fonctions de test, ~25-30 subtests

---

### Phase 2: Tests Handlers Players (`api/handlers_players_test.go`) - Priorité 2

**Fichier:** `/home/user/go-odio-api/api/handlers_players_test.go`

Surface d'API la plus critique. Tests pour handlers MPRIS et mapping d'erreurs:

1. **`TestHandleMPRISError`** - Mapping type erreur → code HTTP (CRITIQUE)
   - `PlayerNotFoundError` → 404 Not Found
   - `InvalidBusNameError` → 400 Bad Request
   - `ValidationError` → 400 Bad Request
   - `CapabilityError` → 403 Forbidden
   - `dbusTimeoutError` → 500 Internal Server Error
   - Erreur générique → 500 Internal Server Error
   - Pas d'erreur → 202 Accepted

2. **`TestListPlayersHandler`** - Liste tous les players
   - Succès avec players → array JSON
   - Liste vide → array JSON vide
   - Erreur backend → 500

3. **`TestPlayHandler`, `TestPauseHandler`, `TestPlayPauseHandler`, `TestStopHandler`**
   - Succès → 202 Accepted
   - Player non trouvé → 404
   - Erreur capability → 403 Forbidden
   - BusName invalide → 400

4. **`TestNextHandler`, `TestPreviousHandler`** - Navigation pistes
   - Même pattern que play/pause

5. **`TestSeekHandler`** - Seek avec body JSON
   - Offset positif valide → 202
   - Offset négatif valide (rembobinage) → 202
   - JSON invalide → 400
   - Player non trouvé → 404

6. **`TestSetPositionHandler`** - Set position avec body
   - trackID + position valides → 202
   - JSON invalide → 400
   - Erreur validation → 400

7. **`TestSetVolumeHandler`** - Set volume avec body
   - Volume valide (0.0-1.0) → 202
   - Volume > 1.0 → 400 erreur validation
   - Volume < 0.0 → 400 erreur validation
   - JSON invalide → 400

8. **`TestSetLoopHandler`, `TestSetShuffleHandler`**
   - Succès → 202
   - Cas d'erreur → codes status appropriés

**Pattern de mock:**
```go
type mockMPRISBackend struct {
    listPlayersFunc func() ([]mpris.Player, error)
    playFunc        func(string) error
    // ... comportement customisable par test
}
```

**Estimé:** ~600-800 lignes, 13 fonctions de test, ~40-50 subtests

---

### Phase 3: Tests Handlers Audio (`api/handlers_audio_test.go`) - Priorité 3

**Fichier:** `/home/user/go-odio-api/api/handlers_audio_test.go`

Handlers de contrôle PulseAudio:

1. **`TestMuteClientHandler`**
   - Succès → 202
   - Sink manquant → 404
   - Erreur backend → 500

2. **`TestMuteMasterHandler`**
   - Succès → 202
   - Erreur backend → 500

3. **`TestSetVolumeClientHandler`**
   - Volume valide → 202
   - Volume hors plage → 400
   - Sink manquant → 404
   - JSON invalide → 400

4. **`TestSetVolumeMasterHandler`**
   - Même structure que volume client

**Pattern de mock:**
```go
type mockPulseAudioBackend struct {
    toggleMuteFunc      func(string) error
    setVolumeFunc       func(string, float32) error
    // ...
}
```

**Estimé:** ~300-400 lignes, 4 fonctions de test, ~15-20 subtests

---

### Phase 4: Tests Handlers Services (`api/handlers_services_test.go`) - Priorité 4

**Fichier:** `/home/user/go-odio-api/api/handlers_services_test.go`

Handlers de contrôle services systemd et Bluetooth:

1. **Handlers de contrôle services** - `TestEnableServiceHandler`, `TestDisableServiceHandler`, `TestStartServiceHandler`, `TestStopServiceHandler`, `TestRestartServiceHandler`
   - Scope system → 202
   - Scope user → 202
   - Scope invalide → 404
   - Unit manquant → 404
   - Erreur backend → 500

2. **`TestListServicesHandler`**
   - Succès avec services → array JSON
   - Liste vide → array vide
   - Erreur backend → 500

3. **Handlers Bluetooth** - `TestBluetoothPowerUpHandler`, `TestBluetoothPowerDownHandler`, `TestBluetoothPairingModeHandler`
   - Succès → 202
   - Erreur backend → 500

**Patterns de mock:**
```go
type mockSystemdBackend struct {
    enableServiceFunc func(string, systemd.UnitScope) error
    // ...
}

type mockBluetoothBackend struct {
    powerUpFunc func() error
    // ...
}
```

**Estimé:** ~400-500 lignes, 10 fonctions de test, ~30-35 subtests

---

## Pattern de Structure de Test

Tous les tests suivent le **pattern table-driven** établi dans le codebase:

```go
func TestHandlerName(t *testing.T) {
    tests := []struct {
        name           string
        setupMock      func() *mockBackend
        requestBody    string
        pathParams     map[string]string
        wantStatusCode int
        wantBodyMatch  string
    }{
        {
            name: "cas succès",
            setupMock: func() *mockBackend {
                return &mockBackend{
                    methodFunc: func() error { return nil },
                }
            },
            wantStatusCode: http.StatusAccepted,
        },
        // ... plus de cas de test
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            mock := tt.setupMock()
            handler := HandlerName(mock)

            req := httptest.NewRequest("POST", "/path", strings.NewReader(tt.requestBody))
            req.SetPathValue("param", tt.pathParams["param"])
            w := httptest.NewRecorder()

            handler(w, req)

            if w.Code != tt.wantStatusCode {
                t.Errorf("status = %d, want %d", w.Code, tt.wantStatusCode)
            }
        })
    }
}
```

---

## Fichiers Critiques

**Fichiers de référence:**
- `/home/user/go-odio-api/api/json_test.go` - Pattern et style de test existants
- `/home/user/go-odio-api/api/json.go` - Implémentation middleware `withBody`
- `/home/user/go-odio-api/api/players.go` - Implémentations handlers et gestion erreurs
- `/home/user/go-odio-api/backend/mpris/errors.go` - Types d'erreurs pour mapping HTTP

**Fichiers à créer:**
- `/home/user/go-odio-api/api/middleware_test.go`
- `/home/user/go-odio-api/api/handlers_players_test.go`
- `/home/user/go-odio-api/api/handlers_audio_test.go`
- `/home/user/go-odio-api/api/handlers_services_test.go`

---

## Vérification

Après implémentation, vérifier avec:

```bash
# Lancer tous les tests API
go test ./api/... -v

# Vérifier la couverture
go test -cover ./api/...

# Lancer un fichier de test spécifique
go test ./api/middleware_test.go -v

# S'assurer pas de panics au démarrage
go build && ./go-odio-api
```

**Critères de succès:**
- Tous les tests passent
- Couverture >85% pour le package API
- Pas de panics au démarrage
- Mapping type erreur vérifié avec tous les codes HTTP
- Tous les cas limites de middleware couverts

---

## Scope Total

- **4 nouveaux fichiers de test**
- **33 fonctions de test**
- **110-135 subtests**
- **~1600-2100 lignes de code de test**
- **Couverture cible: >85%**
