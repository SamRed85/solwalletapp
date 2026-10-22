<template>
  <v-container class="py-6">
    <v-card>
      <v-card-title class="d-flex align-center">
        <v-icon class="mr-2">mdi-account-group</v-icon>
        Users
      </v-card-title>
      <v-card-subtitle>
        A name can be changed, and a user can be removed, only when they have no coupons on an event still in play.
        Wallet addresses cannot be changed.
      </v-card-subtitle>
      <v-data-table :headers="headers" :items="users" :loading="loading" item-value="walletAddress">
        <template #[`item.displayName`]="{ value }">{{ value || '—' }}</template>
        <template #[`item.walletAddress`]="{ value }">
          <code>{{ value }}</code>
        </template>
        <template #[`item.isAdmin`]="{ value }">
          <v-chip v-if="value" color="warning" size="small" variant="flat">admin</v-chip>
        </template>
        <template #[`item.createdAt`]="{ value }">{{ formatDate(value) }}</template>
        <template #[`item.lastLoginAt`]="{ value }">{{ formatDate(value) }}</template>
        <template #[`item.actions`]="{ item }">
          <v-btn icon="mdi-pencil" size="small" variant="text" title="Edit name" :disabled="!canManage(item)" @click="openEdit(item)" />
          <v-btn
            icon="mdi-delete" size="small" variant="text" color="error" title="Remove user"
            :disabled="!canManage(item) || item.walletAddress === auth.wallet" @click="confirmRemove(item)"
          />
        </template>
      </v-data-table>
    </v-card>

    <v-dialog v-model="editDialog" max-width="480">
      <v-card title="Edit profile name">
        <v-card-text>
          <code class="d-block mb-4">{{ selected?.walletAddress }}</code>
          <v-text-field v-model="editName" label="Profile name" :rules="[nameRule]" counter="32" maxlength="32" />
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn @click="editDialog = false">Cancel</v-btn>
          <v-btn color="primary" variant="flat" :disabled="nameRule(editName) !== true" :loading="saving" @click="saveEdit">Save</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="removeDialog" max-width="480">
      <v-card title="Remove user">
        <v-card-text>
          Remove {{ selected?.displayName || 'this user' }}? Their wallet address and any finished coupons stay in the event history.
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn @click="removeDialog = false">Cancel</v-btn>
          <v-btn color="error" variant="flat" :loading="saving" @click="removeUser">Remove</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </v-container>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { userApi } from '@/services/api'
import { auth } from '@/stores/auth'
import { notify, notifyError } from '@/stores/notify'
import { formatDate } from '@/utils/format'

const headers = [
  { title: 'Name', key: 'displayName' },
  { title: 'Wallet', key: 'walletAddress' },
  { title: 'Active coupons', key: 'activeCoupons' },
  { title: 'Role', key: 'isAdmin', sortable: false },
  { title: 'First sign-in', key: 'createdAt' },
  { title: 'Last sign-in', key: 'lastLoginAt' },
  { title: '', key: 'actions', sortable: false, align: 'end' }
]
const nameRule = value => {
  const name = (value || '').trim().replace(/\s+/g, ' ')
  return name.length >= 2 && name.length <= 32 ? true : 'Use 2 to 32 characters'
}

const users = ref([])
const loading = ref(false)
const saving = ref(false)
const selected = ref(null)
const editName = ref('')
const editDialog = ref(false)
const removeDialog = ref(false)

const canManage = user => user.activeCoupons === 0

async function load () {
  loading.value = true
  try {
    users.value = await userApi.list()
  } catch (error) {
    notifyError(error)
  } finally {
    loading.value = false
  }
}

function openEdit (user) {
  selected.value = user
  editName.value = user.displayName || ''
  editDialog.value = true
}

async function saveEdit () {
  saving.value = true
  try {
    await userApi.update(selected.value.walletAddress, editName.value)
    notify('Profile name updated')
    editDialog.value = false
    await load()
  } catch (error) {
    notifyError(error)
  } finally {
    saving.value = false
  }
}

function confirmRemove (user) {
  selected.value = user
  removeDialog.value = true
}

async function removeUser () {
  saving.value = true
  try {
    await userApi.remove(selected.value.walletAddress)
    notify('User removed')
    removeDialog.value = false
    await load()
  } catch (error) {
    notifyError(error)
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>
