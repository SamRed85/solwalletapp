<template>
  <v-container>
    <v-row>
      <v-col cols="12">
        <v-card>
          <v-card-title class="text-h4 pa-6 warning--text">
            <v-icon large color="warning" class="mr-3">mdi-shield-crown</v-icon>
            Admin Dashboard
          </v-card-title>
          
          <v-card-text>
            <v-tabs v-model="activeTab" centered>
              <v-tab>
                <v-icon left>mdi-calendar-plus</v-icon>
                Events
              </v-tab>
              <v-tab>
                <v-icon left>mdi-account-group</v-icon>
                Users
              </v-tab>
            </v-tabs>

            <v-tabs-items v-model="activeTab">
              <!-- Events Tab -->
              <v-tab-item>
                <v-container>
                  <v-row>
                    <v-col cols="12">
                      <div class="d-flex justify-space-between align-center mb-4">
                        <h2 class="text-h5">Event Management</h2>
                        <v-btn color="primary" @click="showCreateEventDialog = true">
                          <v-icon left>mdi-plus</v-icon>
                          Create Event
                        </v-btn>
                      </div>

                      <!-- Events List -->
                      <v-data-table
                        :headers="eventHeaders"
                        :items="events"
                        :loading="loadingEvents"
                        class="elevation-1"
                      >
                        <template v-slot:item.status="{ item }">
                          <v-chip
                            :color="getStatusColor(item.status)"
                            small
                            text-color="white"
                          >
                            {{ item.status }}
                          </v-chip>
                        </template>
                        
                        <template v-slot:item.actions="{ item }">
                          <v-btn
                            small
                            color="primary"
                            @click="viewEvent(item)"
                            class="mr-2"
                          >
                            <v-icon small>mdi-eye</v-icon>
                          </v-btn>
                          <v-btn
                            v-if="item.status === 'upcoming'"
                            small
                            color="success"
                            @click="addGamesToEvent(item)"
                            class="mr-2"
                          >
                            <v-icon small>mdi-plus</v-icon>
                          </v-btn>
                          <v-btn
                            v-if="item.status === 'active'"
                            small
                            color="warning"
                            @click="setEventOutcomes(item)"
                          >
                            <v-icon small>mdi-check</v-icon>
                          </v-btn>
                        </template>
                      </v-data-table>
                    </v-col>
                  </v-row>
                </v-container>
              </v-tab-item>

              <!-- Users Tab -->
              <v-tab-item>
                <v-container>
                  <v-row>
                    <v-col cols="12">
                      <h2 class="text-h5 mb-4">User Management</h2>
                      <v-btn color="primary" @click="$router.push('/users')">
                        <v-icon left>mdi-account-group</v-icon>
                        Manage Users
                      </v-btn>
                    </v-col>
                  </v-row>
                </v-container>
              </v-tab-item>
            </v-tabs-items>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <!-- Create Event Dialog -->
    <v-dialog v-model="showCreateEventDialog" max-width="600px">
      <v-card>
        <v-card-title>
          <span class="text-h5">Create New Event</span>
        </v-card-title>
        
        <v-card-text>
          <v-form ref="eventForm" v-model="eventFormValid">
            <v-text-field
              v-model="newEvent.title"
              label="Event Title"
              :rules="[v => !!v || 'Title is required']"
              required
            ></v-text-field>
            
            <v-textarea
              v-model="newEvent.description"
              label="Event Description"
              rows="3"
            ></v-textarea>
            
            <v-text-field
              v-model.number="newEvent.gameCount"
              label="Number of Games"
              type="number"
              min="7"
              max="13"
              :rules="[
                v => !!v || 'Game count is required',
                v => v >= 7 || 'Minimum 7 games required',
                v => v <= 13 || 'Maximum 13 games allowed'
              ]"
              required
            ></v-text-field>
            
            <v-text-field
              v-model="newEvent.startTime"
              label="Start Time"
              type="datetime-local"
              :rules="[v => !!v || 'Start time is required']"
              required
            ></v-text-field>
            
            <v-text-field
              v-model="newEvent.endTime"
              label="End Time"
              type="datetime-local"
              :rules="[v => !!v || 'End time is required']"
              required
            ></v-text-field>
          </v-form>
        </v-card-text>
        
        <v-card-actions>
          <v-spacer></v-spacer>
          <v-btn color="grey" text @click="showCreateEventDialog = false">
            Cancel
          </v-btn>
          <v-btn
            color="primary"
            :disabled="!eventFormValid"
            :loading="creatingEvent"
            @click="createEvent"
          >
            Create Event
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- Add Games Dialog -->
    <v-dialog v-model="showAddGamesDialog" max-width="800px">
      <v-card>
        <v-card-title>
          <span class="text-h5">Add Games to Event</span>
        </v-card-title>
        
        <v-card-text>
          <v-form ref="gamesForm" v-model="gamesFormValid">
            <div v-for="(game, index) in newGames" :key="index" class="mb-4">
              <v-card outlined>
                <v-card-text>
                  <div class="text-h6 mb-2">Game {{ index + 1 }}</div>
                  <v-row>
                    <v-col cols="5">
                      <v-text-field
                        v-model="game.homeTeam"
                        label="Home Team"
                        :rules="[v => !!v || 'Home team is required']"
                        required
                      ></v-text-field>
                    </v-col>
                    <v-col cols="2" class="d-flex align-center justify-center">
                      <span class="text-h6">VS</span>
                    </v-col>
                    <v-col cols="5">
                      <v-text-field
                        v-model="game.awayTeam"
                        label="Away Team"
                        :rules="[v => !!v || 'Away team is required']"
                        required
                      ></v-text-field>
                    </v-col>
                  </v-row>
                </v-card-text>
              </v-card>
            </div>
          </v-form>
        </v-card-text>
        
        <v-card-actions>
          <v-spacer></v-spacer>
          <v-btn color="grey" text @click="showAddGamesDialog = false">
            Cancel
          </v-btn>
          <v-btn
            color="primary"
            :disabled="!gamesFormValid"
            :loading="addingGames"
            @click="submitGames"
          >
            Add Games
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- Set Outcomes Dialog -->
    <v-dialog v-model="showSetOutcomesDialog" max-width="800px">
      <v-card>
        <v-card-title>
          <span class="text-h5">Set Event Outcomes</span>
        </v-card-title>
        
        <v-card-text>
          <div v-for="game in selectedEventGames" :key="game.id" class="mb-4">
            <v-card outlined>
              <v-card-text>
                <div class="text-h6 mb-2">{{ game.homeTeam }} VS {{ game.awayTeam }}</div>
                <v-radio-group v-model="gameOutcomes[game.id]" row>
                  <v-radio
                    label="Home Win"
                    value="home"
                    color="primary"
                  ></v-radio>
                  <v-radio
                    label="Draw"
                    value="draw"
                    color="warning"
                  ></v-radio>
                  <v-radio
                    label="Away Win"
                    value="away"
                    color="success"
                  ></v-radio>
                </v-radio-group>
              </v-card-text>
            </v-card>
          </div>
        </v-card-text>
        
        <v-card-actions>
          <v-spacer></v-spacer>
          <v-btn color="grey" text @click="showSetOutcomesDialog = false">
            Cancel
          </v-btn>
          <v-btn
            color="primary"
            :loading="settingOutcomes"
            @click="setEventOutcomes"
          >
            Set Outcomes
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- Snackbar -->
    <v-snackbar
      v-model="snackbar.show"
      :color="snackbar.color"
      :timeout="snackbar.timeout"
    >
      {{ snackbar.text }}
      <template v-slot:action="{ attrs }">
        <v-btn
          color="white"
          text
          v-bind="attrs"
          @click="snackbar.show = false"
        >
          Close
        </v-btn>
      </template>
    </v-snackbar>
  </v-container>
</template>

<script>
export default {
  name: 'Admin',
  data() {
    return {
      activeTab: 0,
      loadingEvents: false,
      creatingEvent: false,
      addingGames: false,
      settingOutcomes: false,
      
      // Events
      events: [],
      eventHeaders: [
        { text: 'Title', value: 'title' },
        { text: 'Status', value: 'status' },
        { text: 'Games', value: 'gameCount' },
        { text: 'Created', value: 'createdAt' },
        { text: 'Actions', value: 'actions', sortable: false }
      ],
      
      // Create Event Dialog
      showCreateEventDialog: false,
      eventFormValid: false,
      newEvent: {
        title: '',
        description: '',
        gameCount: 7,
        startTime: '',
        endTime: ''
      },
      
      // Add Games Dialog
      gamesFormValid: false,
      
      // Add Games Dialog
      showAddGamesDialog: false,
      selectedEvent: null,
      newGames: [],
      
      // Set Outcomes Dialog
      showSetOutcomesDialog: false,
      selectedEventGames: [],
      gameOutcomes: {},
      
      // Snackbar
      snackbar: {
        show: false,
        text: '',
        color: 'success',
        timeout: 5000
      }
    }
  },
  
  async mounted() {
    await this.verifyAdminStatus()
    await this.loadEvents()
  },
  
  methods: {
    // Safe navigation helper
    safeNavigate(path) {
      if (this.$route.path !== path) {
        this.$router.push(path).catch(err => {
          if (err.name !== 'NavigationDuplicated') {
            console.error('Navigation error:', err)
          }
        })
      }
    },

    async verifyAdminStatus() {
      try {
        const { adminService } = await import('@/services/api')
        const adminStatus = await adminService.checkAdminStatus()
        
        if (!adminStatus.isAdmin) {
          this.safeNavigate('/')
        }
      } catch (error) {
        console.error('Admin verification failed:', error)
        this.safeNavigate('/')
      }
    },

    async loadEvents() {
      this.loadingEvents = true
      try {
        const { eventService } = await import('@/services/api')
        this.events = await eventService.getEvents()
      } catch (error) {
        this.showSnackbar('Failed to load events: ' + error.message, 'error')
      } finally {
        this.loadingEvents = false
      }
    },

    async createEvent() {
      this.creatingEvent = true
      try {
        const { eventService } = await import('@/services/api')
        
        // Convert datetime-local values to ISO strings
        const eventData = {
          ...this.newEvent,
          startTime: new Date(this.newEvent.startTime).toISOString(),
          endTime: new Date(this.newEvent.endTime).toISOString()
        }
        
        await eventService.createEvent(eventData)
        
        this.showSnackbar('Event created successfully!', 'success')
        this.showCreateEventDialog = false
        this.resetNewEvent()
        await this.loadEvents()
      } catch (error) {
        this.showSnackbar('Failed to create event: ' + error.message, 'error')
        console.error('Event creation error:', error)
      } finally {
        this.creatingEvent = false
      }
    },

    resetNewEvent() {
      this.newEvent = {
        title: '',
        description: '',
        gameCount: 7,
        startTime: '',
        endTime: ''
      }
    },

    addGamesToEvent(event) {
      this.selectedEvent = event
      this.newGames = []
      
      // Initialize games array based on event game count
      for (let i = 0; i < event.gameCount; i++) {
        this.newGames.push({
          homeTeam: '',
          awayTeam: ''
        })
      }
      
      this.showAddGamesDialog = true
    },

    async submitGames() {
      // Validate form first
      if (!this.gamesFormValid) {
        this.showSnackbar('Please fill in all required fields', 'error')
        return
      }
      
      this.addingGames = true
      try {
        const { eventService } = await import('@/services/api')
        
        // Use the correct ID field (MongoDB uses _id)
        const eventId = this.selectedEvent._id || this.selectedEvent.id
        if (!eventId) {
          throw new Error('Event ID not found')
        }
        
        
        await eventService.createGames(eventId, this.newGames)
        
        this.showSnackbar('Games added successfully!', 'success')
        this.showAddGamesDialog = false
        await this.loadEvents()
      } catch (error) {
        this.showSnackbar('Failed to add games: ' + error.message, 'error')
        console.error('Add games error:', error)
      } finally {
        this.addingGames = false
      }
    },

    async viewEvent(event) {
      try {
        const { eventService } = await import('@/services/api')
        const eventId = event._id || event.id
        if (!eventId) {
          throw new Error('Event ID not found')
        }
        
        const eventData = await eventService.getEvent(eventId)
        
        // Show event details in a dialog or navigate to event view
        this.showSnackbar(`Event: ${eventData.event.title}`, 'info')
      } catch (error) {
        this.showSnackbar('Failed to load event details: ' + error.message, 'error')
        console.error('View event error:', error)
      }
    },

    async setEventOutcomes(event) {
      try {
        const { eventService } = await import('@/services/api')
        const eventId = event._id || event.id
        if (!eventId) {
          throw new Error('Event ID not found')
        }
        
        const eventData = await eventService.getEvent(eventId)
        
        this.selectedEvent = event
        this.selectedEventGames = eventData.games
        this.gameOutcomes = {}
        
        // Initialize outcomes for each game
        eventData.games.forEach(game => {
          const gameId = game._id || game.id
          this.gameOutcomes[gameId] = ''
        })
        
        this.showSetOutcomesDialog = true
      } catch (error) {
        this.showSnackbar('Failed to load event games: ' + error.message, 'error')
        console.error('Set outcomes error:', error)
      }
    },

    async setEventOutcomes() {
      this.settingOutcomes = true
      try {
        const { eventService } = await import('@/services/api')
        
        // Get the correct event ID
        const eventId = this.selectedEvent._id || this.selectedEvent.id
        if (!eventId) {
          throw new Error('Event ID not found')
        }
        
        // Prepare game outcomes
        const gameOutcomes = Object.keys(this.gameOutcomes).map(gameId => ({
          gameId,
          outcome: this.gameOutcomes[gameId]
        }))
        
        await eventService.setEventOutcomes(eventId, gameOutcomes)
        
        this.showSnackbar('Event outcomes set successfully!', 'success')
        this.showSetOutcomesDialog = false
        await this.loadEvents()
      } catch (error) {
        this.showSnackbar('Failed to set outcomes: ' + error.message, 'error')
        console.error('Set outcomes error:', error)
      } finally {
        this.settingOutcomes = false
      }
    },

    getStatusColor(status) {
      switch (status) {
        case 'upcoming': return 'blue'
        case 'active': return 'green'
        case 'completed': return 'grey'
        default: return 'grey'
      }
    },

    showSnackbar(text, color = 'success') {
      this.snackbar.text = text
      this.snackbar.color = color
      this.snackbar.show = true
    }
  }
}
</script>