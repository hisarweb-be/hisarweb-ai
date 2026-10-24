/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { ChevronDownIcon } from 'lucide-react'
import * as React from 'react'
import { enUS, fr, ja, ru, tr, vi, zhCN, zhTW } from 'react-day-picker/locale'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Calendar } from '@/components/ui/calendar'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import dayjs from '@/lib/dayjs'
import { formatCalendarDate } from '@/lib/time'
import { cn } from '@/lib/utils'

const calendarLocales = {
  en: enUS,
  zhCN,
  zhTW,
  fr,
  ru,
  ja,
  tr,
  vi,
} as const

// 24-hour clock only: HH:mm or HH:mm:ss. A native time input would follow the
// browser locale and could show AM/PM.
const TIME_PATTERN = /^([01]\d|2[0-3]):[0-5]\d(:[0-5]\d)?$/

interface DateTimePickerProps {
  value?: Date
  onChange?: (date: Date | undefined) => void
  placeholder?: string
  className?: string
}

export function DateTimePicker({
  value,
  onChange,
  placeholder,
  className,
}: DateTimePickerProps) {
  const { t, i18n } = useTranslation()
  const placeholderText = placeholder ?? t('Select date')
  const calendarLocale =
    calendarLocales[i18n.language as keyof typeof calendarLocales] ?? enUS
  const currentYear = new Date().getFullYear()
  const [open, setOpen] = React.useState(false)
  const [date, setDate] = React.useState<Date | undefined>(value)
  const [month, setMonth] = React.useState<Date | undefined>(value)
  const [time, setTime] = React.useState<string>('00:00:00')
  const [draft, setDraft] = React.useState<string | null>(null)

  React.useEffect(() => {
    setDate(value)
    setMonth(value)
    if (value) {
      setTime(dayjs(value).format('HH:mm:ss'))
    }
  }, [value])

  const handleDateSelect = (selectedDate: Date | undefined) => {
    if (selectedDate) {
      const [hours, minutes, seconds = 0] = time.split(':').map(Number)
      const newDate = new Date(selectedDate)
      newDate.setHours(hours, minutes, seconds, 0)
      setDate(newDate)
      setMonth(newDate)
      onChange?.(newDate)
      setOpen(false)
    } else {
      setDate(undefined)
      setMonth(undefined)
      onChange?.(undefined)
    }
  }

  const handleTimeChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const newTime = e.target.value
    // An emptied field keeps the current time: the field shows it again and no
    // invalid date goes out.
    if (!newTime) return
    // Half-typed text stays in the field until it is a complete 24-hour time.
    if (!TIME_PATTERN.test(newTime)) {
      setDraft(newTime)
      return
    }
    setDraft(null)
    setTime(newTime)

    if (date) {
      // A browser may omit :00 seconds from the value.
      const [hours, minutes, seconds = 0] = newTime.split(':').map(Number)
      const newDate = new Date(date)
      newDate.setHours(hours, minutes, seconds, 0)
      setDate(newDate)
      onChange?.(newDate)
    }
  }

  const handleClear = () => {
    setDate(undefined)
    setMonth(undefined)
    setTime('00:00:00')
    onChange?.(undefined)
  }

  return (
    <div className={cn('flex gap-2', className)}>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger
          render={
            <Button
              variant='outline'
              className={cn(
                'flex-1 justify-between font-normal',
                !date && 'text-muted-foreground'
              )}
            />
          }
        >
          {date ? formatCalendarDate(date) : placeholderText}
          <ChevronDownIcon className='h-4 w-4 opacity-50' />
        </PopoverTrigger>
        <PopoverContent className='w-auto overflow-hidden p-0' align='start'>
          <Calendar
            mode='single'
            selected={date}
            month={month}
            onMonthChange={setMonth}
            captionLayout='dropdown'
            onSelect={handleDateSelect}
            locale={calendarLocale}
            startMonth={new Date(currentYear - 100, 0)}
            endMonth={new Date(currentYear + 100, 11)}
          />
        </PopoverContent>
      </Popover>
      <Input
        type='text'
        inputMode='numeric'
        placeholder='HH:mm:ss'
        maxLength={8}
        value={draft ?? time}
        aria-label={t('Time')}
        onChange={handleTimeChange}
        onBlur={() => setDraft(null)}
        className='w-32'
        disabled={!date}
      />
      {date && (
        <Button
          type='button'
          variant='outline'
          size='icon'
          onClick={handleClear}
          className='shrink-0'
          aria-label='Clear'
        >
          <span aria-hidden='true'>✕</span>
        </Button>
      )}
    </div>
  )
}
