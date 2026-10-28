import { http } from './client'
import type { LoginResponse, UserInfo } from '../types'

export const authApi = {
  login: (username: string, password: string) =>
    http.post<LoginResponse>('/auth/login', { username, password }),
  profile: () => http.get<UserInfo>('/auth/profile'),
  logout: () => http.post<{ logged_out: boolean }>('/auth/logout'),
}
