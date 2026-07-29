import { describe, it, expect } from 'vitest'
import { countByLevel, formatLogTime, parseLogLine } from '../../src/lib/domain/logLine.js'

describe('parseLogLine', () => {
  it('lê o formato JSON do zerolog e separa caller do resto dos campos', () => {
    const line =
      '{"level":"warn","time":"2026-07-29T11:04:03Z","caller":"nyaa.go:41","message":"sem torrent","query":"Alpha 12"}'

    const parsed = parseLogLine(line)

    expect(parsed.level).toBe('warn')
    expect(parsed.message).toBe('sem torrent')
    expect(parsed.caller).toBe('nyaa.go:41')
    // caller vira coluna própria: não pode reaparecer no fim da mensagem.
    expect(parsed.extras).toBe('"query"="Alpha 12"')
    expect(parsed.raw).toBe(line)
  })

  it('não inventa extras quando só há os campos padrão', () => {
    const parsed = parseLogLine('{"level":"info","time":"2026-07-29T11:04:02Z","message":"ok"}')
    expect(parsed.extras).toBeUndefined()
    expect(parsed.caller).toBeUndefined()
  })

  it('serializa objetos aninhados dentro de extras', () => {
    const parsed = parseLogLine('{"level":"info","message":"m","meta":{"a":1}}')
    expect(parsed.extras).toBe('"meta"="{"a":1}"')
  })

  it('trata fatal e panic como error', () => {
    expect(parseLogLine('{"level":"fatal","message":"x"}').level).toBe('error')
    expect(parseLogLine('{"level":"panic","message":"x"}').level).toBe('error')
  })

  it('cai para info quando o nível do JSON é desconhecido ou ausente', () => {
    expect(parseLogLine('{"message":"x"}').level).toBe('info')
    expect(parseLogLine('{"level":"trace","message":"x"}').level).toBe('info')
  })

  it('lê o formato console do zerolog pelas siglas de nível', () => {
    const parsed = parseLogLine('11:04AM WRN sem seeds')
    expect(parsed.level).toBe('warn')
    expect(parsed.message).toBe('11:04AM WRN sem seeds')
    expect(parsed.time).toBe('')
  })

  it('mapeia FAT do console para error', () => {
    expect(parseLogLine('11:04AM FAT morreu').level).toBe('error')
  })

  it('usa heurística de palavra-chave numa linha sem formato conhecido', () => {
    expect(parseLogLine('something failed with error 500').level).toBe('error')
    expect(parseLogLine('warning: disco quase cheio').level).toBe('warn')
    expect(parseLogLine('uma linha qualquer').level).toBe('info')
  })

  it('não quebra com JSON malformado — cai para os heurísticos', () => {
    const parsed = parseLogLine('{"level":"error", isto não é json')
    expect(parsed.level).toBe('error')
    expect(parsed.raw).toBe('{"level":"error", isto não é json')
  })
})

describe('formatLogTime', () => {
  it('formata um timestamp ISO como HH:MM:SS', () => {
    // Construído a partir de um Date local para o teste não depender do fuso da máquina.
    const iso = new Date(2026, 6, 29, 9, 5, 3).toISOString()
    expect(formatLogTime(iso)).toBe('09:05:03')
  })

  it('devolve string vazia quando não há timestamp', () => {
    expect(formatLogTime(undefined)).toBe('')
    expect(formatLogTime('')).toBe('')
  })

  it('devolve o valor cru quando não é uma data válida, em vez de "Invalid Date"', () => {
    expect(formatLogTime('nem-data')).toBe('nem-data')
  })
})

describe('countByLevel', () => {
  it('conta por nível e mantém o total em all', () => {
    const parsed = [
      '{"level":"info","message":"a"}',
      '{"level":"error","message":"b"}',
      '{"level":"error","message":"c"}',
      '{"level":"debug","message":"d"}',
    ].map(parseLogLine)

    expect(countByLevel(parsed)).toEqual({ all: 4, debug: 1, info: 1, warn: 0, error: 2 })
  })

  it('zera tudo numa lista vazia', () => {
    expect(countByLevel([])).toEqual({ all: 0, debug: 0, info: 0, warn: 0, error: 0 })
  })
})
